package db

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// 本文件覆盖主进程侧的取消通道（Issue #1329）：agent 声明支持在途取消时，
// 上下文取消必须先发取消通知并保留连接；只有 agent 不响应宽限期时才回退到终止传输。

// cancelChannelAgentStub 是一个可控的 agent 替身：读到查询请求后保持挂起，
// 直到收到取消通知（或测试显式放行/关闭 stdout）才给出响应。
type cancelChannelAgentStub struct {
	stdinMu    sync.Mutex
	requests   []optionalAgentRequest
	reply      chan []byte
	replyOnce  sync.Once
	cancelSeen chan int64
	closed     chan struct{}
}

func newCancelChannelAgentStub() *cancelChannelAgentStub {
	return &cancelChannelAgentStub{
		reply:      make(chan []byte, 4),
		cancelSeen: make(chan int64, 4),
		closed:     make(chan struct{}),
	}
}

func (s *cancelChannelAgentStub) Write(payload []byte) (int, error) {
	line := strings.TrimSpace(string(payload))
	if line == "" {
		return len(payload), nil
	}
	var req optionalAgentRequest
	if err := json.Unmarshal([]byte(line), &req); err != nil {
		return 0, err
	}
	s.stdinMu.Lock()
	s.requests = append(s.requests, req)
	s.stdinMu.Unlock()

	if req.Method == optionalAgentMethodCancelQuery {
		s.cancelSeen <- req.TargetID
		// 取消通知按约定不回帧：agent 侧的中止效果体现在被取消请求的响应里。
		return len(payload), nil
	}
	return len(payload), nil
}

func (s *cancelChannelAgentStub) recordedRequests() []optionalAgentRequest {
	s.stdinMu.Lock()
	defer s.stdinMu.Unlock()
	return append([]optionalAgentRequest(nil), s.requests...)
}

func (s *cancelChannelAgentStub) Close() error {
	s.replyOnce.Do(func() { close(s.closed) })
	return nil
}

// respondQuery 让挂起的查询以失败响应收尾，模拟 agent 中止语句后的回帧。
func (s *cancelChannelAgentStub) respondQuery(t *testing.T, requestID int64, errText string) {
	t.Helper()
	payload, err := json.Marshal(optionalAgentResponse{ID: requestID, Success: false, Error: errText})
	if err != nil {
		t.Fatalf("构造查询响应失败：%v", err)
	}
	s.reply <- append(payload, '\n')
}

// cancelChannelStdout 把 stub 的回帧与显式关闭串成 io.ReadCloser，供 client.reader 使用。
type cancelChannelStdout struct {
	stub *cancelChannelAgentStub
	mu   sync.Mutex
	buf  []byte
}

func newCancelChannelStdout(stub *cancelChannelAgentStub) *cancelChannelStdout {
	stdout := &cancelChannelStdout{stub: stub}
	go func() {
		for {
			select {
			case payload := <-stub.reply:
				stdout.mu.Lock()
				stdout.buf = append(stdout.buf, payload...)
				stdout.mu.Unlock()
			case <-stub.closed:
				return
			}
		}
	}()
	return stdout
}

func (s *cancelChannelStdout) Read(p []byte) (int, error) {
	for {
		s.mu.Lock()
		if len(s.buf) > 0 {
			n := copy(p, s.buf)
			s.buf = s.buf[n:]
			s.mu.Unlock()
			return n, nil
		}
		s.mu.Unlock()

		select {
		case <-s.stub.closed:
			return 0, io.EOF
		case <-time.After(2 * time.Millisecond):
		}
	}
}

func (s *cancelChannelStdout) Close() error { return nil }

func newCancelChannelClient(stub *cancelChannelAgentStub, inFlightCancel bool) *optionalDriverAgentClient {
	client := &optionalDriverAgentClient{
		stdin:  stub,
		stdout: newCancelChannelStdout(stub),
		driver: "sqlserver",
	}
	client.reader = bufio.NewReader(client.stdout)
	client.setConnectionCapabilities(optionalAgentConnectionInfo{InFlightCancel: inFlightCancel})
	return client
}

// TestOptionalDriverAgentCancelNotifiesAgentAndKeepsTransport 是 Issue #1329 的主进程侧回归：
// agent 支持在途取消时，取消查询必须先发取消通知、保留 transport（连接可继续用），
// 而不是像旧行为那样杀掉 agent 进程。
func TestOptionalDriverAgentCancelNotifiesAgentAndKeepsTransport(t *testing.T) {
	// 宽限期足够长，确保断言的是通知路径而不是回退终止。
	previousGrace := optionalAgentInFlightCancelGrace
	optionalAgentInFlightCancelGrace = 30 * time.Second
	t.Cleanup(func() { optionalAgentInFlightCancelGrace = previousGrace })

	stub := newCancelChannelAgentStub()
	defer stub.Close()
	client := newCancelChannelClient(stub, true)
	dbInst := &OptionalDriverAgentDB{driverType: "sqlserver", client: client}

	ctx, cancel := context.WithCancel(context.Background())
	queryDone := make(chan error, 1)
	go func() {
		_, _, err := dbInst.QueryContext(ctx, "SELECT slow")
		queryDone <- err
	}()

	// 等查询请求真正下发（此时它已登记为在途请求），再取消。
	deadline := time.Now().Add(5 * time.Second)
	for len(stub.recordedRequests()) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("查询请求未下发")
		}
		time.Sleep(2 * time.Millisecond)
	}
	requests := stub.recordedRequests()
	queryRequestID := requests[0].ID
	cancel()

	select {
	case targetID := <-stub.cancelSeen:
		if targetID != queryRequestID {
			t.Fatalf("取消通知目标 = %d，want 在途请求 %d", targetID, queryRequestID)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("查询执行期间未收到取消通知（仍走杀进程路径）")
	}

	// agent 中止语句后回帧；transport 必须保持可用。
	if err := client.stoppedError(); err != nil {
		t.Fatalf("发送取消通知后 transport 不应被终止：%v", err)
	}
	stub.respondQuery(t, queryRequestID, "context canceled")

	select {
	case err := <-queryDone:
		if err == nil {
			t.Fatal("被取消的查询应返回错误")
		}
		if !strings.Contains(strings.ToLower(err.Error()), "context canceled") {
			t.Fatalf("取消错误文本 = %q，期望包含 context canceled", err.Error())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("取消后查询未返回")
	}

	// 取消通知按约定不回帧，且不得让 transport 进入停止态：连接仍可继续使用。
	if err := client.stoppedError(); err != nil {
		t.Fatalf("取消后连接应保持可用：%v", err)
	}
	notifications := 0
	for _, req := range stub.recordedRequests() {
		if req.Method == optionalAgentMethodCancelQuery {
			notifications++
		}
	}
	if notifications != 1 {
		t.Fatalf("取消通知条数 = %d，want 1", notifications)
	}
}

// TestOptionalDriverAgentCancelFallsBackToTerminateOnOldAgent 旧版 agent 未声明取消能力时，
// 必须保持「终止传输」的既有行为，而不是发出它无法理解的取消通知。
func TestOptionalDriverAgentCancelFallsBackToTerminateOnOldAgent(t *testing.T) {
	stub := newCancelChannelAgentStub()
	defer stub.Close()
	client := newCancelChannelClient(stub, false)
	dbInst := &OptionalDriverAgentDB{driverType: "sqlserver", client: client}

	ctx, cancel := context.WithCancel(context.Background())
	queryDone := make(chan error, 1)
	go func() {
		_, _, err := dbInst.QueryContext(ctx, "SELECT slow")
		queryDone <- err
	}()

	deadline := time.Now().Add(5 * time.Second)
	for len(stub.recordedRequests()) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("查询请求未下发")
		}
		time.Sleep(2 * time.Millisecond)
	}
	cancel()

	select {
	case err := <-queryDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("旧 agent 上取消应返回 context.Canceled，got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("旧 agent 上取消未及时收回请求")
	}
	if err := client.stoppedError(); err == nil {
		t.Fatal("旧 agent 不支持取消通道时应终止 transport")
	}
	for _, req := range stub.recordedRequests() {
		if req.Method == optionalAgentMethodCancelQuery {
			t.Fatalf("不应向未声明能力的 agent 发送取消通知：%#v", req)
		}
	}
}

// TestOptionalDriverAgentCancelFallsBackWhenAgentIgnoresNotification agent 收到通知却不收尾时，
// 必须在宽限期后回退到终止传输，避免 UI 永久卡在「正在停止」。
func TestOptionalDriverAgentCancelFallsBackWhenAgentIgnoresNotification(t *testing.T) {
	previousGrace := optionalAgentInFlightCancelGrace
	optionalAgentInFlightCancelGrace = 30 * time.Millisecond
	t.Cleanup(func() { optionalAgentInFlightCancelGrace = previousGrace })

	stub := newCancelChannelAgentStub()
	defer stub.Close()
	client := newCancelChannelClient(stub, true)
	dbInst := &OptionalDriverAgentDB{driverType: "sqlserver", client: client}

	ctx, cancel := context.WithCancel(context.Background())
	queryDone := make(chan error, 1)
	go func() {
		_, _, err := dbInst.QueryContext(ctx, "SELECT slow")
		queryDone <- err
	}()

	deadline := time.Now().Add(5 * time.Second)
	for len(stub.recordedRequests()) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("查询请求未下发")
		}
		time.Sleep(2 * time.Millisecond)
	}
	cancel()

	select {
	case targetID := <-stub.cancelSeen:
		if targetID <= 0 {
			t.Fatalf("取消通知缺少目标请求 ID：%d", targetID)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("未发出取消通知")
	}

	select {
	case err := <-queryDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("宽限期后应回退终止并返回 context.Canceled，got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("agent 未收尾时宽限期回退未生效")
	}
	if err := client.stoppedError(); err == nil {
		t.Fatal("宽限期超时后应终止 transport")
	}
}

// TestOptionalDriverAgentDeadlineKeepsImmediateTermination 超时（DeadlineExceeded）不走取消通知：
// agent 已按 TimeoutMs 派生自己的死线，保持既有的即时代理回收延迟。
func TestOptionalDriverAgentDeadlineKeepsImmediateTermination(t *testing.T) {
	previousGrace := optionalAgentInFlightCancelGrace
	optionalAgentInFlightCancelGrace = 30 * time.Second
	t.Cleanup(func() { optionalAgentInFlightCancelGrace = previousGrace })

	stub := newCancelChannelAgentStub()
	defer stub.Close()
	client := newCancelChannelClient(stub, true)
	dbInst := &OptionalDriverAgentDB{driverType: "sqlserver", client: client}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	startedAt := time.Now()
	_, _, err := dbInst.QueryContext(ctx, "SELECT slow")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("超时应返回 DeadlineExceeded，got %v", err)
	}
	if elapsed := time.Since(startedAt); elapsed > 2*time.Second {
		t.Fatalf("超时后未及时回收 agent 进程：%s", elapsed)
	}
	if err := client.stoppedError(); err == nil {
		t.Fatal("超时后应终止 transport")
	}
	for _, req := range stub.recordedRequests() {
		if req.Method == optionalAgentMethodCancelQuery {
			t.Fatal("超时不应发送取消通知")
		}
	}
}

// TestOptionalDriverAgentCancelSkipsUnsupportedSchemaViaConnectionInfo 能力门控读取 connect 响应：
// 未回显 inFlightCancel 的旧 agent 一律走终止路径。
func TestOptionalDriverAgentCancelSkipsUnsupportedSchemaViaConnectionInfo(t *testing.T) {
	client := &optionalDriverAgentClient{driver: "sqlserver"}
	client.setConnectionCapabilities(optionalAgentConnectionInfo{ProtocolSchema: OptionalDriverAgentProtocolSchemaV2})
	if client.supportsInFlightCancel() {
		t.Fatal("旧 agent（仅 v2 协议）不应被判定为支持在途取消")
	}
	if client.schema() != OptionalDriverAgentProtocolSchemaV2 {
		t.Fatalf("协议版本 = %q，want %q", client.schema(), OptionalDriverAgentProtocolSchemaV2)
	}

	client.setConnectionCapabilities(optionalAgentConnectionInfo{
		ProtocolSchema: OptionalDriverAgentProtocolSchemaV2,
		InFlightCancel: true,
	})
	if !client.supportsInFlightCancel() {
		t.Fatal("声明 inFlightCancel 的 agent 应被判定为支持在途取消")
	}
}
