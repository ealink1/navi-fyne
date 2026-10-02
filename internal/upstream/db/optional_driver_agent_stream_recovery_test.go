package db

import (
	"bufio"
	"context"
	"errors"
	"io"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// errStreamConsumerCanceled 模拟导出/前端取消等消费端主动中断。
var errStreamConsumerCanceled = errors.New("consumer canceled")

func streamTestFrameID(id int64) string {
	return strconv.FormatInt(id, 10)
}

// streamTestCancelingConsumer 在第 consumeLimit 行后返回取消错误。
type streamTestCancelingConsumer struct {
	columns       []string
	rows          [][]interface{}
	consumeLimit  int
	consumeCalled int
}

func (c *streamTestCancelingConsumer) SetColumns(columns []string) error {
	c.columns = append([]string(nil), columns...)
	return nil
}

func (c *streamTestCancelingConsumer) ConsumeRow(row map[string]interface{}) error {
	c.consumeCalled++
	if c.consumeCalled > c.consumeLimit {
		return errStreamConsumerCanceled
	}
	values := make([]interface{}, len(c.columns))
	for idx, column := range c.columns {
		values[idx] = row[column]
	}
	c.rows = append(c.rows, values)
	return nil
}

// streamTestFailingSetColumns 模拟列设置阶段失败（如导出器初始化失败）。
type streamTestFailingSetColumns struct {
	columns []string
}

func (c *streamTestFailingSetColumns) SetColumns(columns []string) error {
	return errStreamConsumerCanceled
}

func (c *streamTestFailingSetColumns) ConsumeRow(row map[string]interface{}) error {
	return errStreamConsumerCanceled
}

func streamTestRequestFrames(id int64, rowsData string) []string {
	return []string{
		`{"id":` + streamTestFrameID(id) + `,"success":true,"chunkType":"columns","fields":["id","name"]}`,
		`{"id":` + streamTestFrameID(id) + `,"success":true,"chunkType":"rows","data":` + rowsData + `}`,
	}
}

func streamTestDoneFrame(id int64) string {
	return `{"id":` + streamTestFrameID(id) + `,"success":true,"chunkType":"done"}`
}

func TestOptionalDriverAgentClientStreamDrainsResidueAfterConsumeRowError(t *testing.T) {
	var stdin optionalAgentTestWriteCloser
	// 请求 1 中途取消后，agent 仍会写完剩余行 + done；随后串行处理请求 2。
	stdout := strings.Join([]string{
		streamTestRequestFrames(1, `[[1,"alice"],[2,"bob"]]`)[0],
		streamTestRequestFrames(1, `[[1,"alice"],[2,"bob"]]`)[1],
		`{"id":1,"success":true,"chunkType":"rows","data":[[3,"carol"]]}`,
		streamTestDoneFrame(1),
		streamTestRequestFrames(2, `[[9,"dave"]]`)[0],
		streamTestRequestFrames(2, `[[9,"dave"]]`)[1],
		streamTestDoneFrame(2),
	}, "\n") + "\n"

	client := &optionalDriverAgentClient{
		stdin:  &stdin,
		reader: bufio.NewReader(strings.NewReader(stdout)),
		driver: "oceanbase",
	}
	consumer := &streamTestCancelingConsumer{consumeLimit: 0}
	err := client.callStreamQuery(optionalAgentRequest{
		Method: optionalAgentMethodStreamQuery,
		Query:  "SELECT 1",
	}, consumer)
	if !errors.Is(err, errStreamConsumerCanceled) {
		t.Fatalf("应返回消费端原始错误，got %v", err)
	}
	if len(consumer.rows) != 0 || consumer.consumeCalled != 1 {
		t.Fatalf("取消前应只处理第 1 行: rows=%d called=%d", len(consumer.rows), consumer.consumeCalled)
	}

	// 排空后 transport 仍可用，且请求 2 读到的是自己的帧而不是请求 1 的残留行。
	next := &optionalAgentTestStreamConsumer{}
	if err := client.callStreamQuery(optionalAgentRequest{
		Method: optionalAgentMethodStreamQuery,
		Query:  "SELECT 2",
	}, next); err != nil {
		t.Fatalf("排空后下一个请求应正常完成: %v", err)
	}
	if len(next.rows) != 1 || next.rows[0][0] != int64(9) {
		t.Fatalf("下一个请求读到残留帧或数据异常: %#v", next.rows)
	}
	if strings.Join(next.columns, ",") != "id,name" {
		t.Fatalf("下一个请求列信息异常: %#v", next.columns)
	}
}

func TestOptionalDriverAgentClientStreamDrainsAfterSetColumnsError(t *testing.T) {
	var stdin optionalAgentTestWriteCloser
	stdout := strings.Join([]string{
		streamTestRequestFrames(1, `[[1,"alice"]]`)[0],
		streamTestRequestFrames(1, `[[1,"alice"]]`)[1],
		streamTestDoneFrame(1),
		streamTestRequestFrames(2, `[[9,"dave"]]`)[0],
		streamTestRequestFrames(2, `[[9,"dave"]]`)[1],
		streamTestDoneFrame(2),
	}, "\n") + "\n"

	client := &optionalDriverAgentClient{
		stdin:  &stdin,
		reader: bufio.NewReader(strings.NewReader(stdout)),
		driver: "oceanbase",
	}
	err := client.callStreamQuery(optionalAgentRequest{
		Method: optionalAgentMethodStreamQuery,
		Query:  "SELECT 1",
	}, &streamTestFailingSetColumns{})
	if !errors.Is(err, errStreamConsumerCanceled) {
		t.Fatalf("应返回消费端原始错误，got %v", err)
	}

	next := &optionalAgentTestStreamConsumer{}
	if err := client.callStreamQuery(optionalAgentRequest{
		Method: optionalAgentMethodStreamQuery,
		Query:  "SELECT 2",
	}, next); err != nil {
		t.Fatalf("排空后下一个请求应正常完成: %v", err)
	}
	if len(next.rows) != 1 || next.rows[0][0] != int64(9) {
		t.Fatalf("下一个请求读到残留帧或数据异常: %#v", next.rows)
	}
}

func TestOptionalDriverAgentClientStreamDrainsAfterRowDecodeError(t *testing.T) {
	var stdin optionalAgentTestWriteCloser
	stdout := strings.Join([]string{
		streamTestRequestFrames(1, `"not-an-array"`)[0],
		streamTestRequestFrames(1, `"not-an-array"`)[1],
		streamTestDoneFrame(1),
		streamTestRequestFrames(2, `[[9,"dave"]]`)[0],
		streamTestRequestFrames(2, `[[9,"dave"]]`)[1],
		streamTestDoneFrame(2),
	}, "\n") + "\n"

	client := &optionalDriverAgentClient{
		stdin:  &stdin,
		reader: bufio.NewReader(strings.NewReader(stdout)),
		driver: "oceanbase",
	}
	err := client.callStreamQuery(optionalAgentRequest{
		Method: optionalAgentMethodStreamQuery,
		Query:  "SELECT 1",
	}, &optionalAgentTestStreamConsumer{})
	if err == nil || !strings.Contains(err.Error(), "解析") {
		t.Fatalf("应返回行解码错误，got %v", err)
	}

	next := &optionalAgentTestStreamConsumer{}
	if err := client.callStreamQuery(optionalAgentRequest{
		Method: optionalAgentMethodStreamQuery,
		Query:  "SELECT 2",
	}, next); err != nil {
		t.Fatalf("排空后下一个请求应正常完成: %v", err)
	}
	if len(next.rows) != 1 || next.rows[0][0] != int64(9) {
		t.Fatalf("下一个请求读到残留帧或数据异常: %#v", next.rows)
	}
}

func TestOptionalDriverAgentClientStreamDrainsUnknownChunkType(t *testing.T) {
	var stdin optionalAgentTestWriteCloser
	stdout := strings.Join([]string{
		streamTestRequestFrames(1, `[[1,"alice"]]`)[0],
		`{"id":1,"success":true,"chunkType":"progress","data":null}`,
		streamTestRequestFrames(1, `[[1,"alice"]]`)[1],
		streamTestDoneFrame(1),
		streamTestRequestFrames(2, `[[9,"dave"]]`)[0],
		streamTestRequestFrames(2, `[[9,"dave"]]`)[1],
		streamTestDoneFrame(2),
	}, "\n") + "\n"

	client := &optionalDriverAgentClient{
		stdin:  &stdin,
		reader: bufio.NewReader(strings.NewReader(stdout)),
		driver: "oceanbase",
	}
	err := client.callStreamQuery(optionalAgentRequest{
		Method: optionalAgentMethodStreamQuery,
		Query:  "SELECT 1",
	}, &optionalAgentTestStreamConsumer{})
	if err == nil || !strings.Contains(err.Error(), "未知流式分片类型") {
		t.Fatalf("应返回未知分片类型错误，got %v", err)
	}

	next := &optionalAgentTestStreamConsumer{}
	if err := client.callStreamQuery(optionalAgentRequest{
		Method: optionalAgentMethodStreamQuery,
		Query:  "SELECT 2",
	}, next); err != nil {
		t.Fatalf("排空后下一个请求应正常完成: %v", err)
	}
	if len(next.rows) != 1 || next.rows[0][0] != int64(9) {
		t.Fatalf("下一个请求读到残留帧或数据异常: %#v", next.rows)
	}
}

func TestOptionalDriverAgentClientStreamTerminatesOnResponseIDMismatch(t *testing.T) {
	var stdin optionalAgentTestWriteCloser
	stdout := strings.Join([]string{
		streamTestRequestFrames(1, `[[1,"alice"]]`)[0],
		`{"id":2,"success":true,"chunkType":"rows","data":[[1,"alice"]]}`,
		streamTestDoneFrame(1),
	}, "\n") + "\n"

	client := &optionalDriverAgentClient{
		stdin:  &stdin,
		reader: bufio.NewReader(strings.NewReader(stdout)),
		driver: "oceanbase",
	}
	err := client.callStreamQuery(optionalAgentRequest{
		Method: optionalAgentMethodStreamQuery,
		Query:  "SELECT 1",
	}, &optionalAgentTestStreamConsumer{})
	if err == nil || !strings.Contains(err.Error(), "ID 不匹配") {
		t.Fatalf("应返回响应 ID 不匹配错误，got %v", err)
	}
	if client.stoppedError() == nil {
		t.Fatal("ID 不匹配后 transport 应被立即终止")
	}

	nextErr := client.callStreamQuery(optionalAgentRequest{
		Method: optionalAgentMethodStreamQuery,
		Query:  "SELECT 2",
	}, &optionalAgentTestStreamConsumer{})
	if nextErr == nil || !strings.Contains(nextErr.Error(), "传输不可用") {
		t.Fatalf("终止后的请求应快速失败且不得读取管道，got %v", nextErr)
	}
}

func TestOptionalDriverAgentClientStreamTerminatesOnMissingDone(t *testing.T) {
	var stdin optionalAgentTestWriteCloser
	// 流未写 done 即结束（EOF）：无法确认剩余帧边界，必须回收 transport。
	stdout := strings.Join(streamTestRequestFrames(1, `[[1,"alice"]]`), "\n") + "\n"

	client := &optionalDriverAgentClient{
		stdin:  &stdin,
		reader: bufio.NewReader(strings.NewReader(stdout)),
		driver: "oceanbase",
	}
	err := client.callStreamQuery(optionalAgentRequest{
		Method: optionalAgentMethodStreamQuery,
		Query:  "SELECT 1",
	}, &optionalAgentTestStreamConsumer{})
	if err == nil || !strings.Contains(err.Error(), "读取") {
		t.Fatalf("应返回读取失败错误，got %v", err)
	}
	if client.stoppedError() == nil {
		t.Fatal("缺少 done 时 transport 应被终止")
	}

	nextErr := client.callStreamQuery(optionalAgentRequest{
		Method: optionalAgentMethodStreamQuery,
		Query:  "SELECT 2",
	}, &optionalAgentTestStreamConsumer{})
	if nextErr == nil || !strings.Contains(nextErr.Error(), "传输不可用") {
		t.Fatalf("终止后的请求应快速失败，got %v", nextErr)
	}
}

func TestOptionalDriverAgentClientStreamTerminatesOnIDMismatchDuringDrain(t *testing.T) {
	var stdin optionalAgentTestWriteCloser
	// 排空期间出现其他请求的帧：帧边界已不可信，必须终止 transport。
	stdout := strings.Join([]string{
		streamTestRequestFrames(1, `[[1,"alice"]]`)[0],
		streamTestRequestFrames(1, `[[1,"alice"]]`)[1],
		streamTestRequestFrames(2, `[[9,"dave"]]`)[0],
		streamTestDoneFrame(1),
	}, "\n") + "\n"

	client := &optionalDriverAgentClient{
		stdin:  &stdin,
		reader: bufio.NewReader(strings.NewReader(stdout)),
		driver: "oceanbase",
	}
	consumer := &streamTestCancelingConsumer{consumeLimit: 0}
	err := client.callStreamQuery(optionalAgentRequest{
		Method: optionalAgentMethodStreamQuery,
		Query:  "SELECT 1",
	}, consumer)
	if !errors.Is(err, errStreamConsumerCanceled) {
		t.Fatalf("应返回消费端原始错误，got %v", err)
	}
	if client.stoppedError() == nil {
		t.Fatal("排空期间 ID 不匹配应终止 transport")
	}
}

func TestOptionalDriverAgentClientStreamValidatesDoneFrameID(t *testing.T) {
	var stdin optionalAgentTestWriteCloser
	// done 帧同样必须校验 ID，防止把上一个请求的 done 当作本请求的完成信号。
	stdout := strings.Join([]string{
		streamTestRequestFrames(1, `[[1,"alice"]]`)[0],
		streamTestRequestFrames(1, `[[1,"alice"]]`)[1],
		streamTestDoneFrame(2),
	}, "\n") + "\n"

	client := &optionalDriverAgentClient{
		stdin:  &stdin,
		reader: bufio.NewReader(strings.NewReader(stdout)),
		driver: "oceanbase",
	}
	err := client.callStreamQuery(optionalAgentRequest{
		Method: optionalAgentMethodStreamQuery,
		Query:  "SELECT 1",
	}, &optionalAgentTestStreamConsumer{})
	if err == nil || !strings.Contains(err.Error(), "ID 不匹配") {
		t.Fatalf("done 帧 ID 不匹配应报错，got %v", err)
	}
	if client.stoppedError() == nil {
		t.Fatal("done 帧 ID 不匹配后 transport 应被终止")
	}
}

func TestOptionalDriverAgentClientStreamValueConsumerErrorDrainsResidue(t *testing.T) {
	var stdin optionalAgentTestWriteCloser
	stdout := strings.Join([]string{
		streamTestRequestFrames(1, `[[1,"alice"],[2,"bob"]]`)[0],
		streamTestRequestFrames(1, `[[1,"alice"],[2,"bob"]]`)[1],
		streamTestDoneFrame(1),
		streamTestRequestFrames(2, `[[9,"dave"]]`)[0],
		streamTestRequestFrames(2, `[[9,"dave"]]`)[1],
		streamTestDoneFrame(2),
	}, "\n") + "\n"

	client := &optionalDriverAgentClient{
		stdin:  &stdin,
		reader: bufio.NewReader(strings.NewReader(stdout)),
		driver: "oceanbase",
	}
	consumer := &streamTestCancelingValueConsumer{consumeLimit: 0}
	err := client.callStreamQuery(optionalAgentRequest{
		Method: optionalAgentMethodStreamQuery,
		Query:  "SELECT 1",
	}, consumer)
	if !errors.Is(err, errStreamConsumerCanceled) {
		t.Fatalf("应返回消费端原始错误，got %v", err)
	}

	next := &optionalAgentTestStreamConsumer{}
	if err := client.callStreamQuery(optionalAgentRequest{
		Method: optionalAgentMethodStreamQuery,
		Query:  "SELECT 2",
	}, next); err != nil {
		t.Fatalf("排空后下一个请求应正常完成: %v", err)
	}
	if len(next.rows) != 1 || next.rows[0][0] != int64(9) {
		t.Fatalf("下一个请求读到残留帧或数据异常: %#v", next.rows)
	}
}

type streamTestCancelingValueConsumer struct {
	columns       []string
	rows          [][]interface{}
	consumeLimit  int
	consumeCalled int
}

func (c *streamTestCancelingValueConsumer) SetColumns(columns []string) error {
	c.columns = append([]string(nil), columns...)
	return nil
}

func (c *streamTestCancelingValueConsumer) ConsumeRowValues(values []interface{}) error {
	c.consumeCalled++
	if c.consumeCalled > c.consumeLimit {
		return errStreamConsumerCanceled
	}
	c.rows = append(c.rows, append([]interface{}(nil), values...))
	return nil
}

func (c *streamTestCancelingValueConsumer) ConsumeRow(row map[string]interface{}) error {
	return errStreamConsumerCanceled
}

func TestOptionalDriverAgentClientStreamTerminatesOnMalformedFrame(t *testing.T) {
	var stdin optionalAgentTestWriteCloser
	stdout := strings.Join([]string{
		streamTestRequestFrames(1, `[[1,"alice"]]`)[0],
		`not-json`,
	}, "\n") + "\n"

	client := &optionalDriverAgentClient{
		stdin:  &stdin,
		reader: bufio.NewReader(strings.NewReader(stdout)),
		driver: "oceanbase",
	}
	err := client.callStreamQuery(optionalAgentRequest{
		Method: optionalAgentMethodStreamQuery,
		Query:  "SELECT 1",
	}, &optionalAgentTestStreamConsumer{})
	if err == nil || !strings.Contains(err.Error(), "解析") {
		t.Fatalf("应返回帧解码失败错误，got %v", err)
	}
	if client.stoppedError() == nil {
		t.Fatal("帧解码失败后 transport 应被终止")
	}

	nextErr := client.callStreamQuery(optionalAgentRequest{
		Method: optionalAgentMethodStreamQuery,
		Query:  "SELECT 2",
	}, &optionalAgentTestStreamConsumer{})
	if nextErr == nil || !strings.Contains(nextErr.Error(), "传输不可用") {
		t.Fatalf("终止后的请求应快速失败，got %v", nextErr)
	}
}

func TestOptionalDriverAgentClientStreamDrainsWhenRowsArriveBeforeColumns(t *testing.T) {
	var stdin optionalAgentTestWriteCloser
	stdout := strings.Join([]string{
		`{"id":1,"success":true,"chunkType":"rows","data":[[1,"alice"]]}`,
		streamTestDoneFrame(1),
		streamTestRequestFrames(2, `[[9,"dave"]]`)[0],
		streamTestRequestFrames(2, `[[9,"dave"]]`)[1],
		streamTestDoneFrame(2),
	}, "\n") + "\n"

	client := &optionalDriverAgentClient{
		stdin:  &stdin,
		reader: bufio.NewReader(strings.NewReader(stdout)),
		driver: "oceanbase",
	}
	err := client.callStreamQuery(optionalAgentRequest{
		Method: optionalAgentMethodStreamQuery,
		Query:  "SELECT 1",
	}, &optionalAgentTestStreamConsumer{})
	if err == nil || !strings.Contains(err.Error(), "缺少列信息") {
		t.Fatalf("应返回缺少列信息错误，got %v", err)
	}
	if client.stoppedError() != nil {
		t.Fatalf("协议性分片异常走排空路径，transport 不应被终止: %v", client.stoppedError())
	}

	next := &optionalAgentTestStreamConsumer{}
	if err := client.callStreamQuery(optionalAgentRequest{
		Method: optionalAgentMethodStreamQuery,
		Query:  "SELECT 2",
	}, next); err != nil {
		t.Fatalf("排空后下一个请求应正常完成: %v", err)
	}
	if len(next.rows) != 1 || next.rows[0][0] != int64(9) {
		t.Fatalf("下一个请求读到残留帧或数据异常: %#v", next.rows)
	}
}

// streamTestCancellablePipeReader 先回放预置帧，之后阻塞直到 Close 关闭通道，
// 用于确定性地模拟“排空卡在无剩余帧的管道上，随后被 ctx 取消打断”。
type streamTestCancellablePipeReader struct {
	mu     sync.Mutex
	frames []byte
	block  chan struct{}
	closed bool
}

func newStreamTestCancellablePipeReader(frames string) *streamTestCancellablePipeReader {
	return &streamTestCancellablePipeReader{
		frames: []byte(frames),
		block:  make(chan struct{}),
	}
}

func (r *streamTestCancellablePipeReader) Read(p []byte) (int, error) {
	r.mu.Lock()
	if len(r.frames) > 0 {
		pending := r.frames
		r.frames = nil
		r.mu.Unlock()
		return copy(p, pending), nil
	}
	r.mu.Unlock()
	<-r.block
	return 0, io.ErrClosedPipe
}

func (r *streamTestCancellablePipeReader) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.closed {
		r.closed = true
		close(r.block)
	}
	return nil
}

func TestOptionalDriverAgentClientStreamDrainInterruptedByContextCancel(t *testing.T) {
	pipe := newStreamTestCancellablePipeReader(strings.Join([]string{
		streamTestRequestFrames(1, `[[1,"alice"]]`)[0],
		streamTestRequestFrames(1, `[[1,"alice"]]`)[1],
	}, "\n") + "\n")
	var stdin optionalAgentTestWriteCloser

	client := &optionalDriverAgentClient{
		stdin:  &stdin,
		stdout: pipe,
		reader: bufio.NewReader(pipe),
		driver: "oceanbase",
	}

	// 消费端在第 1 行中止 → 排空阻塞在空管道上 → ctx 超时触发
	// AfterFunc forceTerminate 关闭管道 → 排空读失败返回。
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	err := client.callStreamQueryContext(ctx, optionalAgentRequest{
		Method: optionalAgentMethodStreamQuery,
		Query:  "SELECT 1",
	}, &streamTestCancelingConsumer{consumeLimit: 0})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("应返回 ctx 超时错误，got %v", err)
	}
	if client.stoppedError() == nil {
		t.Fatal("ctx 取消打断排空后 transport 应被终止")
	}

	nextErr := client.callStreamQuery(optionalAgentRequest{
		Method: optionalAgentMethodStreamQuery,
		Query:  "SELECT 2",
	}, &optionalAgentTestStreamConsumer{})
	if nextErr == nil || !strings.Contains(nextErr.Error(), "传输不可用") {
		t.Fatalf("终止后的请求应快速失败，got %v", nextErr)
	}
}
