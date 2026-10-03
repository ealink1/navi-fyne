package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ealink1/super-link/internal/upstream/connection"
	"github.com/ealink1/super-link/internal/upstream/db"
)

// 本文件覆盖「停止查询」的服务端取消通道（Issue #1329）：
//
//	用户点停止 → 主进程发取消通知 → agent 取消在途请求的 context → 驱动层中止语句
//	→ 连接保持可用，而不是杀掉 agent 进程。

const cancelTestWait = 5 * time.Second

// blockingAgentQueryDB 是「语句已下发、服务端仍在执行」的驱动替身：查询阻塞到 context
// 被取消才返回 context.Canceled，复现真实驱动（如 go-mssqldb 发送 TDS attention 后）的
// 收尾形态。其余 Database 方法复用 fakeAgentTimeoutDB。
type blockingAgentQueryDB struct {
	fakeAgentTimeoutDB
	entered     chan struct{}
	ctxErrOnEnd error
	pingCalls   int
	mu          sync.Mutex
}

func (f *blockingAgentQueryDB) Ping() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pingCalls++
	return nil
}

func (f *blockingAgentQueryDB) QueryContext(ctx context.Context, _ string) ([]map[string]interface{}, []string, error) {
	close(f.entered)
	<-ctx.Done()
	f.ctxErrOnEnd = ctx.Err()
	return nil, nil, ctx.Err()
}

func (f *blockingAgentQueryDB) QueryContextWithMessages(ctx context.Context, query string) ([]map[string]interface{}, []string, []string, error) {
	data, fields, err := f.QueryContext(ctx, query)
	return data, fields, nil, err
}

// cancelTestInput 是可增量喂入的请求输入。测试需要在「查询已进入驱动之后」再写入取消
// 通知，因此不能用一次性构造好的固定 reader。
type cancelTestInput struct {
	mu      sync.Mutex
	cond    *sync.Cond
	pending []byte
	closed  bool
}

func newCancelTestInput() *cancelTestInput {
	input := &cancelTestInput{}
	input.cond = sync.NewCond(&input.mu)
	return input
}

func (i *cancelTestInput) push(line []byte) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.pending = append(i.pending, append(line, '\n')...)
	i.cond.Broadcast()
}

func (i *cancelTestInput) Close() error {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.closed = true
	i.cond.Broadcast()
	return nil
}

func (i *cancelTestInput) Read(p []byte) (int, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	for len(i.pending) == 0 && !i.closed {
		i.cond.Wait()
	}
	if len(i.pending) == 0 {
		return 0, io.EOF
	}
	n := copy(p, i.pending)
	i.pending = i.pending[n:]
	return n, nil
}

// cancelTestOutput 是并发安全的响应收集器：serve 循环在别的 goroutine 里写，
// 测试 goroutine 需要随时读取已收到的帧。
type cancelTestOutput struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (o *cancelTestOutput) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.buf.Write(p)
}

func (o *cancelTestOutput) text() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.buf.String()
}

func marshalCancelTestRequest(t *testing.T, req agentRequest) []byte {
	t.Helper()
	payload, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("构造请求失败：%v", err)
	}
	return payload
}

// waitCancelTestResponses 等待响应帧数量达到 want。
func waitCancelTestResponses(t *testing.T, out *cancelTestOutput, want int) []agentResponse {
	t.Helper()
	deadline := time.Now().Add(cancelTestWait)
	for {
		text := out.text()
		if responses := decodeAgentResponses(t, []byte(text)); len(responses) >= want {
			return responses
		}
		if time.Now().After(deadline) {
			t.Fatalf("等待 %d 条响应超时，已收到：%s", want, out.text())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestServeAgentRequestsCancelsInFlightQueryWithinGrace 锁定 Issue #1329 的核心链路：
// 查询执行期间收到取消通知，执行中的语句必须看到 context 取消并立即收尾，且连接随后
// 仍然可用（而不是杀掉 agent 进程）。
func TestServeAgentRequestsCancelsInFlightQueryWithinGrace(t *testing.T) {
	fake := &blockingAgentQueryDB{entered: make(chan struct{})}
	runtimeState := &agentRuntime{inst: fake, sessions: make(map[string]db.StatementExecer)}

	input := newCancelTestInput()
	t.Cleanup(func() { _ = input.Close() })
	out := &cancelTestOutput{}
	writer := bufio.NewWriter(out)

	serveDone := make(chan error, 1)
	go func() {
		serveDone <- serveAgentRequests(input, writer, runtimeState)
	}()

	input.push(marshalCancelTestRequest(t, agentRequest{ID: 41, Method: agentMethodQuery, Query: "SELECT slow"}))
	select {
	case <-fake.entered:
	case <-time.After(cancelTestWait):
		t.Fatal("查询未进入驱动，无法覆盖执行期间取消")
	}

	input.push(marshalCancelTestRequest(t, agentRequest{ID: 42, Method: agentMethodCancelQuery, TargetID: 41}))

	responses := waitCancelTestResponses(t, out, 1)
	if responses[0].ID != 41 || responses[0].Success {
		t.Fatalf("被取消的查询应返回失败响应：%#v", responses[0])
	}
	if !strings.Contains(strings.ToLower(responses[0].Error), "context canceled") {
		t.Fatalf("查询失败原因应来自 context 取消：%q", responses[0].Error)
	}
	if !errors.Is(fake.ctxErrOnEnd, context.Canceled) {
		t.Fatalf("驱动收到的取消原因 = %v，want context.Canceled", fake.ctxErrOnEnd)
	}

	// 取消通知不回帧：紧随其后的 ping 必须收到自己的成功响应，证明 agent 与连接仍然可用。
	input.push(marshalCancelTestRequest(t, agentRequest{ID: 43, Method: agentMethodPing}))
	responses = waitCancelTestResponses(t, out, 2)
	if responses[1].ID != 43 || !responses[1].Success {
		t.Fatalf("取消后连接应继续可用，ping 响应 = %#v", responses[1])
	}
	if fake.pingCalls != 1 {
		t.Fatalf("ping 调用次数 = %d，want 1", fake.pingCalls)
	}

	if err := input.Close(); err != nil {
		t.Fatalf("关闭请求输入失败：%v", err)
	}
	select {
	case err := <-serveDone:
		if err != nil {
			t.Fatalf("serveAgentRequests 返回错误：%v", err)
		}
	case <-time.After(cancelTestWait):
		t.Fatal("serveAgentRequests 未退出")
	}
}

// TestServeAgentRequestsIgnoresCancelForFinishedRequest 迟到或无目标的取消通知不得影响后续请求。
func TestServeAgentRequestsIgnoresCancelForFinishedRequest(t *testing.T) {
	runtimeState := &agentRuntime{sessions: make(map[string]db.StatementExecer)}
	metadataLine := marshalCancelTestRequest(t, agentRequest{ID: 2, Method: agentMethodMetadata})
	input := bytes.NewReader(bytes.Join([][]byte{
		marshalCancelTestRequest(t, agentRequest{ID: 1, Method: agentMethodCancelQuery, TargetID: 999}),
		metadataLine,
	}, []byte{'\n'}))

	var out bytes.Buffer
	writer := bufio.NewWriter(&out)
	if err := serveAgentRequests(input, writer, runtimeState); err != nil {
		t.Fatalf("serveAgentRequests 返回错误：%v", err)
	}
	if err := writer.Flush(); err != nil {
		t.Fatalf("Flush 失败：%v", err)
	}

	responses := decodeAgentResponses(t, out.Bytes())
	if len(responses) != 1 {
		t.Fatalf("取消通知不得单独回帧，响应条数 = %d：%s", len(responses), out.String())
	}
	if responses[0].ID != 2 || !responses[0].Success {
		t.Fatalf("无关取消通知影响了后续请求：%#v", responses[0])
	}
}

func TestAgentRequestCancellerMatchesOnlyInFlightRequest(t *testing.T) {
	canceller := &agentRequestCanceller{}
	cancelled := 0
	generation := canceller.register(7, func() { cancelled++ })
	defer canceller.unregister(7, generation)

	if canceller.cancelRequest(8) {
		t.Fatal("取消通知命中了非在途请求")
	}
	if cancelled != 0 {
		t.Fatalf("非在途请求的取消不应触发 cancel：%d", cancelled)
	}
	if !canceller.cancelRequest(7) {
		t.Fatal("在途请求的取消未被接受")
	}
	if cancelled != 1 {
		t.Fatalf("cancel 调用次数 = %d，want 1", cancelled)
	}
}

func TestAgentRequestCancellerKeepsNewRegistrationAfterStaleUnregister(t *testing.T) {
	canceller := &agentRequestCanceller{}
	firstGeneration := canceller.register(1, func() {})
	secondGeneration := canceller.register(2, func() {})

	// 旧请求的收尾不得清除新请求的登记。
	canceller.unregister(1, firstGeneration)
	if !canceller.cancelRequest(2) {
		t.Fatal("旧请求收尾清除了新请求的登记")
	}
	canceller.unregister(2, secondGeneration)
	if canceller.cancelRequest(2) {
		t.Fatal("登记注销后仍被取消")
	}
}

// TestAgentConnectAdvertisesInFlightCancel 能力声明必须随 connect 响应回显，
// 主进程据此才能在「发取消通知」与「杀进程」之间做出选择。
func TestAgentConnectAdvertisesInFlightCancel(t *testing.T) {
	previousFactory := agentDatabaseFactory
	previousDriverType := agentDriverType
	t.Cleanup(func() {
		agentDatabaseFactory = previousFactory
		agentDriverType = previousDriverType
	})
	agentDriverType = "sqlserver"
	agentDatabaseFactory = func() db.Database { return &blockingAgentQueryDB{entered: make(chan struct{})} }

	runtimeState := &agentRuntime{sessions: make(map[string]db.StatementExecer)}
	config := connection.ConnectionConfig{}
	resp := handleRequest(runtimeState, agentRequest{ID: 1, Method: agentMethodConnect, Config: &config})
	if !resp.Success {
		t.Fatalf("connect 失败：%s", resp.Error)
	}
	info, ok := resp.Data.(agentConnectionInfo)
	if !ok {
		t.Fatalf("connect 响应类型 = %T", resp.Data)
	}
	if !info.InFlightCancel {
		t.Fatal("connect 响应未声明支持在途取消")
	}
	if info.ProtocolSchema != agentProtocolSchemaV2 {
		t.Fatalf("协议版本 = %q，want %q", info.ProtocolSchema, agentProtocolSchemaV2)
	}
}

// cancellableCtxProbeDB 记录驱动实际收到的 context 是否可取消。
type cancellableCtxProbeDB struct {
	fakeAgentTimeoutDB
	cancellable bool
}

func (f *cancellableCtxProbeDB) QueryContextWithMessages(ctx context.Context, _ string) ([]map[string]interface{}, []string, []string, error) {
	f.cancellable = ctx.Done() != nil
	return []map[string]interface{}{}, []string{}, nil, nil
}

func (f *cancellableCtxProbeDB) ExecContext(ctx context.Context, _ string) (int64, error) {
	f.cancellable = ctx.Done() != nil
	return 0, nil
}

// TestAgentDriverContextStaysCancellableWithoutTimeout 锁定 Issue #1329 的根因：
// 未配置查询超时时，驱动拿到的 context 原本是 Background（不可取消），因此「停止」
// 无法传递到服务端，只能靠杀掉 agent 进程。现在它必须始终可取消。
func TestAgentDriverContextStaysCancellableWithoutTimeout(t *testing.T) {
	probe := &cancellableCtxProbeDB{}
	runtimeState := &agentRuntime{inst: probe, sessions: make(map[string]db.StatementExecer)}
	requestCtx, cancelRequest := context.WithCancel(context.Background())
	defer cancelRequest()

	resp := handleRequestWithContext(requestCtx, runtimeState, agentRequest{ID: 1, Method: agentMethodQuery, Query: "SELECT 1"}, nil)
	if !resp.Success {
		t.Fatalf("query 失败：%s", resp.Error)
	}
	if !probe.cancellable {
		t.Fatal("无超时配置时驱动 context 不可取消：停止无法传递到服务端")
	}

	probe.cancellable = false
	_ = handleRequestWithContext(requestCtx, runtimeState, agentRequest{ID: 2, Method: agentMethodExec, Query: "UPDATE t SET a = 1"}, nil)
	if !probe.cancellable {
		t.Fatal("无超时配置时写入 context 不可取消：停止无法传递到服务端")
	}
}
