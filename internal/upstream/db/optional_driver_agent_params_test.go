package db

import (
	"bufio"
	"context"
	"errors"
	"strings"
	"testing"
)

func newParamsTestAgentClient(t *testing.T, schema string) (*optionalDriverAgentClient, *optionalAgentTestWriteCloser) {
	t.Helper()
	var stdin optionalAgentTestWriteCloser
	client := &optionalDriverAgentClient{
		stdin:  &stdin,
		reader: bufio.NewReader(strings.NewReader(`{"id":1,"success":true,"data":[],"fields":[]}` + "\n" + `{"id":2,"success":true,"rowsAffected":1}` + "\n")),
		driver: "dameng",
	}
	if schema != "" {
		client.setSchema(schema)
		client.setConnectionCapabilities(optionalAgentConnectionInfo{TypedArguments: schema == OptionalDriverAgentProtocolSchemaV2})
	}
	return client, &stdin
}

func TestOptionalDriverAgentParamsGatedOnOldSchema(t *testing.T) {
	client, stdin := newParamsTestAgentClient(t, "")
	dbInst := &OptionalDriverAgentDB{driverType: "dameng", client: client}

	_, _, err := dbInst.QueryContextWithArgs(context.Background(), "SELECT :x", []any{"v"})
	if !errors.Is(err, ErrOptionalDriverAgentParamsUnsupported) {
		t.Fatalf("旧协议应拒绝参数绑定: %v", err)
	}
	if stdin.String() != "" {
		t.Fatalf("拒绝执行时不得发送任何请求: %q", stdin.String())
	}

	_, err = dbInst.ExecContextWithArgs(context.Background(), "UPDATE t SET a = :x", []any{"v"})
	if !errors.Is(err, ErrOptionalDriverAgentParamsUnsupported) {
		t.Fatalf("旧协议应拒绝参数化写入: %v", err)
	}
	if stdin.String() != "" {
		t.Fatalf("拒绝执行时不得发送任何请求: %q", stdin.String())
	}
}

func TestOptionalDriverAgentParamsSentOnV2Schema(t *testing.T) {
	client, stdin := newParamsTestAgentClient(t, OptionalDriverAgentProtocolSchemaV2)
	dbInst := &OptionalDriverAgentDB{driverType: "dameng", client: client}

	rows, fields, err := dbInst.QueryContextWithArgs(context.Background(), "SELECT id FROM t WHERE a = ? AND b = ?", []any{"v", int64(2)})
	if err != nil {
		t.Fatalf("QueryContextWithArgs 返回错误: %v", err)
	}
	if len(rows) != 0 || len(fields) != 0 {
		t.Fatalf("响应数据异常: %#v %#v", rows, fields)
	}
	payload := stdin.String()
	if !strings.Contains(payload, `"args":["v","2"]`) || !strings.Contains(payload, `"argTypes":["string","integer"]`) {
		t.Fatalf("请求载荷应携带 args 通道: %s", payload)
	}
	if !strings.Contains(payload, `"method":"query"`) {
		t.Fatalf("请求方法异常: %s", payload)
	}

	if _, err := dbInst.ExecContextWithArgs(context.Background(), "UPDATE t SET a = ?", []any{"x"}); err != nil {
		t.Fatalf("ExecContextWithArgs 返回错误: %v", err)
	}
	if !strings.Contains(stdin.String(), `"method":"exec"`) || !strings.Contains(stdin.String(), `"args":["x"]`) {
		t.Fatalf("参数化写入载荷异常: %s", stdin.String())
	}
}

func TestOptionalDriverAgentSessionParamsGatedOnOldSchema(t *testing.T) {
	client, stdin := newParamsTestAgentClient(t, "")
	session := &optionalDriverAgentSession{client: client, driver: "dameng", sessionID: "s1"}

	if _, _, err := session.QueryContextWithArgs(context.Background(), "SELECT :x", []any{"v"}); !errors.Is(err, ErrOptionalDriverAgentParamsUnsupported) {
		t.Fatalf("旧协议会话应拒绝参数绑定: %v", err)
	}
	if _, err := session.ExecContextWithArgs(context.Background(), "UPDATE t SET a = :x", []any{"v"}); !errors.Is(err, ErrOptionalDriverAgentParamsUnsupported) {
		t.Fatalf("旧协议会话应拒绝参数化写入: %v", err)
	}
	if stdin.String() != "" {
		t.Fatalf("拒绝执行时不得发送任何请求: %q", stdin.String())
	}
}

func TestAgentSupportsParameterBinding(t *testing.T) {
	if agentSupportsParameterBinding("") || agentSupportsParameterBinding("json-lines-v1") {
		t.Fatal("v1 及以下协议不应开放参数绑定")
	}
	if !agentSupportsParameterBinding(OptionalDriverAgentProtocolSchemaV2) {
		t.Fatal("v2 协议应开放参数绑定")
	}
}
