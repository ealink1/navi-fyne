// Package chat implements bounded, cancellable OpenAI-compatible text chat.
package chat

import (
	"errors"
	"net"
	"net/url"
	"strings"
	"unicode/utf8"
)

// Config is stored as one encrypted bundle, never as public application metadata.
type Config struct {
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
	APIKey  string `json:"api_key"`
	Stream  bool   `json:"stream"`
}

// Message contains only text explicitly entered into the conversation.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Limits bound both conversation memory and remote protocol parsing.
const (
	MaxInputBytes   = 8 << 10
	MaxContextBytes = 64 << 10
	MaxReplyBytes   = 32 << 10
	MaxMessages     = 40
)

// Validate rejects ambiguous addresses and credentials in URLs.
func (c Config) Validate() error {
	if len(c.BaseURL) > 2048 || len(c.APIKey) > 4096 || len(c.Model) > 128 {
		return errors.New("AI 配置超过长度限制")
	}
	u, err := url.Parse(c.BaseURL)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return errors.New("请填写不含账号、查询参数或片段的 API 基础地址")
	}
	if u.Scheme != "https" && (u.Scheme != "http" || !localHost(u.Hostname())) {
		return errors.New("远端 API 请使用 HTTPS；HTTP 仅支持本机或私有 IP")
	}
	if u.Port() != "" {
		if _, err := net.LookupPort("tcp", u.Port()); err != nil {
			return errors.New("API 地址的端口无效")
		}
	}
	if strings.TrimSpace(c.Model) == "" || !utf8.ValidString(c.Model) || strings.ContainsAny(c.Model+c.APIKey, "\r\n\x00") {
		return errors.New("请填写有效的模型名称和 API Key")
	}
	return nil
}

func localHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && (ip.IsLoopback() || ip.IsPrivate())
}

func endpoint(base string) string {
	base = strings.TrimRight(base, "/")
	if strings.HasSuffix(base, "/chat/completions") {
		return base
	}
	return base + "/chat/completions"
}

func validateMessages(messages []Message) error {
	if len(messages) == 0 || len(messages) > MaxMessages {
		return errors.New("对话消息数量超过限制，请开始新对话")
	}
	size := 0
	for i, m := range messages {
		expected := "user"
		if i%2 == 1 {
			expected = "assistant"
		}
		if m.Role != expected || m.Content == "" || !utf8.ValidString(m.Content) {
			return errors.New("对话消息无效")
		}
		limit := MaxReplyBytes
		if m.Role == "user" {
			limit = MaxInputBytes
		}
		if len(m.Content) > limit {
			return errors.New("单条消息超过长度限制")
		}
		size += len(m.Content)
	}
	if len(messages)%2 != 1 || size > MaxContextBytes {
		return errors.New("对话超过上下文限制，请开始新对话")
	}
	return nil
}
