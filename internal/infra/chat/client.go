package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// Client owns a small pool of HTTP connections. Requests never follow redirects.
type Client struct{ http *http.Client }

// New creates a client with connection, header and total request deadlines.
func New() *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{Timeout: 10 * time.Second}).DialContext
	transport.TLSHandshakeTimeout = 10 * time.Second
	transport.ResponseHeaderTimeout = 45 * time.Second
	transport.IdleConnTimeout = 30 * time.Second
	transport.MaxIdleConnsPerHost = 1
	return &Client{http: &http.Client{Transport: transport, Timeout: 3 * time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

// Close releases idle sockets; active requests belong to the caller's context.
func (c *Client) Close() { c.http.CloseIdleConnections() }

// Reply calls onText synchronously with cumulative text, if streaming is enabled.
// It returns partial text on cancellation or error, but callers must not use an
// incomplete reply as future model context. Error bodies and URLs are not exposed.
func (c *Client) Reply(ctx context.Context, config Config, messages []Message, onText func(string)) (string, error) {
	if err := config.Validate(); err != nil {
		return "", err
	}
	if err := validateMessages(messages); err != nil {
		return "", err
	}
	requestBody, err := json.Marshal(struct {
		Model    string    `json:"model"`
		Messages []Message `json:"messages"`
		Stream   bool      `json:"stream"`
	}{Model: config.Model, Messages: messages, Stream: config.Stream})
	if err != nil {
		return "", errors.New("无法编码 AI 请求")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint(config.BaseURL), bytes.NewReader(requestBody))
	if err != nil {
		return "", errors.New("API 地址无效")
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	if config.APIKey != "" {
		request.Header.Set("Authorization", "Bearer "+config.APIKey)
	}
	response, err := c.http.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return "", errors.New("AI 请求超时，请稍后重试")
		}
		return "", errors.New("无法连接 AI 服务，请检查地址、网络和证书")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", statusError(response.StatusCode)
	}
	if strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "text/event-stream") {
		return readStream(ctx, response.Body, onText)
	}
	return readCompletion(ctx, response.Body)
}

func statusError(status int) error {
	switch status {
	case 401, 403:
		return errors.New("AI 服务拒绝授权，请检查 API Key 和模型权限")
	case 404:
		return errors.New("AI 接口或模型不存在，请检查基础地址和模型名称")
	case 429:
		return errors.New("AI 服务请求受限，请检查额度或稍后重试")
	default:
		return fmt.Errorf("AI 服务返回 HTTP %d，请检查服务状态和接口设置", status)
	}
}

const maxWireBytes = 2 << 20

type completion struct {
	Choices []struct {
		Index   int     `json:"index"`
		Delta   Message `json:"delta"`
		Message Message `json:"message"`
		Finish  *string `json:"finish_reason"`
	} `json:"choices"`
	Error json.RawMessage `json:"error"`
}

func readCompletion(ctx context.Context, reader io.Reader) (string, error) {
	raw, err := io.ReadAll(io.LimitReader(reader, maxWireBytes+1))
	if err != nil {
		return "", responseReadError(ctx)
	}
	if len(raw) > maxWireBytes {
		return "", errors.New("AI 响应超过读取限制")
	}
	var result completion
	if err := json.Unmarshal(raw, &result); err != nil || len(result.Error) > 0 && string(result.Error) != "null" || len(result.Choices) == 0 {
		return "", errors.New("AI 响应格式无效，请检查接口兼容性")
	}
	text := result.Choices[0].Message.Content
	if len(text) > MaxReplyBytes {
		return "", errors.New("AI 回复超过 32 KiB 限制")
	}
	if strings.TrimSpace(text) == "" {
		return "", errors.New("AI 没有返回文字回复，请检查模型是否支持普通问答")
	}
	return text, nil
}

func responseReadError(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return errors.New("AI 回复读取中断或超时，请重试")
}
