package db

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/ealink1/navi-fyne/internal/upstream/logger"
)

// 本文件承载主进程侧的「在途查询取消」通道：agent 声明支持后，上下文取消先发取消
// 通知并保留连接，只有 agent 未在宽限期内收尾时才回退到终止传输（见 Issue #1329）。

// optionalAgentMethodCancelQuery 中止指定的在途请求。它按约定不回帧：取消效果由被取消
// 请求自己的响应体现，因此不能走请求/响应读取循环（见 notifyInFlightCancel）。
const optionalAgentMethodCancelQuery = "cancelQuery"

// optionalAgentInFlightCancelGrace 是发出取消通知后等待 agent 自行中止查询的宽限期：
// 超过它仍未结束才回退到终止传输。测试可缩短该值。
var optionalAgentInFlightCancelGrace = 5 * time.Second

// cancelInFlightRequest 处理请求上下文的取消。
//
// 关键差异：agent 支持在途取消通道时，先发取消通知，让驱动层向服务端发送终止信号
// （如 SQL Server 的 TDS attention）并保留连接；只有 agent 未在宽限期内结束该请求时才
// 回退到终止传输（旧行为）。
//
// 超时（DeadlineExceeded）不走通知：agent 已按 TimeoutMs 派生了自己的死线，会自行中止，
// 客户端无需等待宽限期，保持既有的即时代理回收延迟。
func (c *optionalDriverAgentClient) cancelInFlightRequest(ctx context.Context, requestID int64, operationDone <-chan struct{}) {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || !c.supportsInFlightCancel() {
		_ = c.forceTerminate(ctx.Err())
		return
	}
	if err := c.notifyInFlightCancel(requestID); err != nil {
		logger.Warnf("%s 驱动代理取消通知发送失败，回退到终止传输：err=%v", driverDisplayName(c.driver), err)
		_ = c.forceTerminate(ctx.Err())
		return
	}

	select {
	case <-operationDone:
		// 请求已自行结束：连接与 agent 进程保持可用，后续查询无需重连。
		return
	case <-time.After(optionalAgentInFlightCancelGrace):
	}
	logger.Warnf("%s 驱动代理未在宽限期内响应取消，回退到终止传输：requestID=%d", driverDisplayName(c.driver), requestID)
	_ = c.forceTerminate(ctx.Err())
}

// notifyInFlightCancel 向 agent 发送取消通知。
//
// 该通知按约定不回帧，因此不走请求/响应读取循环：它只写入一帧，且必须能在另一个
// goroutine 正在等待查询响应时写入（见 request_dispatch.go 的 agent 侧实现）。
func (c *optionalDriverAgentClient) notifyInFlightCancel(targetRequestID int64) error {
	if targetRequestID <= 0 {
		return errors.New("取消通知缺少在途请求 ID")
	}
	if err := c.stoppedError(); err != nil {
		return err
	}
	request := optionalAgentRequest{
		ID:       c.nextCancellationRequestID(),
		Method:   optionalAgentMethodCancelQuery,
		TargetID: targetRequestID,
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return err
	}
	payload = append(payload, '\n')

	if err := c.writeRequestFrame(payload); err != nil {
		return err
	}
	return nil
}

func (c *optionalDriverAgentClient) nextCancellationRequestID() int64 {
	c.requestMu.Lock()
	defer c.requestMu.Unlock()
	c.nextRequestID++
	return c.nextRequestID
}

// supportsInFlightCancel 报告 agent 是否声明支持在途取消通道。
func (c *optionalDriverAgentClient) supportsInFlightCancel() bool {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	return c.inFlightCancel
}

func (c *optionalDriverAgentClient) setConnectionCapabilities(info optionalAgentConnectionInfo) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	if c.protocolSchema == "" {
		c.protocolSchema = strings.TrimSpace(info.ProtocolSchema)
	}
	if info.InFlightCancel {
		c.inFlightCancel = true
	}
	c.typedArguments = info.TypedArguments
}

// beginRequest 分配请求 ID 并把它登记为在途请求，返回释放函数。
//
// 登记必须发生在读取响应之前、且在 watcher 安装之前完成，取消通知才能稳定地指向
// 本次请求：否则取消可能在请求尚未登记时到达，导致通知落空。
func (c *optionalDriverAgentClient) beginRequest() (int64, func()) {
	c.requestMu.Lock()
	c.nextRequestID++
	requestID := c.nextRequestID
	c.inFlightRequestID = requestID
	c.requestMu.Unlock()

	return requestID, func() {
		c.requestMu.Lock()
		if c.inFlightRequestID == requestID {
			c.inFlightRequestID = 0
		}
		c.requestMu.Unlock()
	}
}

// writeRequestFrame 串行化写入 stdin：取消通知由 watcher goroutine 发出，
// 可能与持有传输的请求写帧并发。
func (c *optionalDriverAgentClient) writeRequestFrame(payload []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_, err := c.stdin.Write(payload)
	return err
}
