package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func fixtureClient(t *testing.T, handler http.HandlerFunc) (*Client, Config) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client := New()
	t.Cleanup(client.Close)
	return client, Config{BaseURL: server.URL + "/v1", Model: "fixture", APIKey: "test-private-key", Stream: true}
}

func TestReplyWireContractAndJSONFallback(t *testing.T) {
	client, config := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Method != "POST" || r.Header.Get("Authorization") != "Bearer test-private-key" {
			t.Error("request contract mismatch")
		}
		var body struct {
			Model    string          `json:"model"`
			Messages []Message       `json:"messages"`
			Stream   bool            `json:"stream"`
			Tools    json.RawMessage `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Model != "fixture" || !body.Stream || len(body.Messages) != 3 || body.Messages[2].Content != "followup" || len(body.Tools) != 0 {
			t.Error("text-only multi-turn payload mismatch")
		}
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"**你好**"}}]}`)
	})
	result, err := client.Reply(context.Background(), config, []Message{{Role: "user", Content: "hi"}, {Role: "assistant", Content: "hello"}, {Role: "user", Content: "followup"}}, nil)
	if err != nil || result != "**你好**" {
		t.Fatal(result, err)
	}
}

func TestStreamingCumulativeTextAndMultilineEvents(t *testing.T) {
	client, config := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		fmt.Fprint(w, `: ping

data: {"choices":
data: [{"index":0,"delta":{"role":"assistant","content":"你"}}]}

`)
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":1,\"delta\":{\"content\":\"ignored\"}},{\"index\":0,\"delta\":{\"content\":\"好\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	})
	var updates []string
	result, err := client.Reply(context.Background(), config, []Message{{Role: "user", Content: "hi"}}, func(text string) { updates = append(updates, text) })
	if err != nil || result != "你好" || len(updates) != 2 || updates[0] != "你" || updates[1] != "你好" {
		t.Fatal(result, updates, err)
	}
}

func TestCancellationClosesStreamingRequest(t *testing.T) {
	started, stopped := make(chan struct{}), make(chan struct{})
	client, config := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		defer close(stopped)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n")
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := client.Reply(ctx, config, []Message{{Role: "user", Content: "hi"}}, func(string) { cancel() })
		done <- err
	}()
	defer cancel()
	<-started
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancel did not interrupt body reading")
	}
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("server request remained open")
	}
}

func TestRemoteFailuresNeverExposeBodyOrFollowRedirect(t *testing.T) {
	var targetRequests atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { targetRequests.Add(1) }))
	defer target.Close()
	for _, code := range []int{401, 404, 429, 500, 307} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			client, config := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", target.URL)
				w.WriteHeader(code)
				fmt.Fprint(w, "private-user-input test-private-key")
			})
			_, err := client.Reply(context.Background(), config, []Message{{Role: "user", Content: "hi"}}, nil)
			if err == nil || strings.Contains(err.Error(), "private") {
				t.Fatal("remote body leaked", err)
			}
		})
	}
	if targetRequests.Load() != 0 {
		t.Fatal("redirect forwarded credentials")
	}
}

func TestResponseParsingLimitsAndIncompleteStream(t *testing.T) {
	for _, tc := range []struct{ name, kind, body string }{
		{"oversized JSON", "application/json", strings.Repeat("a", maxWireBytes+1)},
		{"oversized reply", "application/json", `{"choices":[{"message":{"content":"` + strings.Repeat("x", MaxReplyBytes+1) + `"}}]}`},
		{"empty", "application/json", `{"choices":[{"message":{"content":""}}]}`},
		{"malformed", "application/json", `private-error-not-json`},
		{"stream dropped", "text/event-stream", "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n"},
		{"stream event too large", "text/event-stream", "data: " + strings.Repeat("x", 129<<10) + "\n\n"},
		{"stream reply too large", "text/event-stream", "data: {\"choices\":[{\"delta\":{\"content\":\"" + strings.Repeat("x", MaxReplyBytes+1) + "\"}}]}\n\ndata: [DONE]\n\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, config := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tc.kind)
				fmt.Fprint(w, tc.body)
			})
			_, err := client.Reply(context.Background(), config, []Message{{Role: "user", Content: "hi"}}, nil)
			if err == nil || strings.Contains(err.Error(), "private-error") {
				t.Fatal(err)
			}
		})
	}
}

func TestConfigAndMessagesValidateBeforeNetwork(t *testing.T) {
	for _, url := range []string{"http://example.com/v1", "https://user:password@example.com/v1", "https://example.com/v1?key=secret", "file:///tmp/data", "https://example.com/#fragment", "https://example.com:999999/v1"} {
		if err := (Config{BaseURL: url, Model: "model"}).Validate(); err == nil {
			t.Errorf("accepted %s", url)
		}
	}
	for _, url := range []string{"https://example.com/v1", "http://localhost:11434/v1", "http://[::1]:8080/v1", "http://192.168.1.20:11434/v1"} {
		if err := (Config{BaseURL: url, Model: "model"}).Validate(); err != nil {
			t.Errorf("rejected %s: %v", url, err)
		}
	}
	for _, messages := range [][]Message{nil, {{Role: "assistant", Content: "x"}}, {{Role: "user", Content: strings.Repeat("x", MaxInputBytes+1)}}, {{Role: "user", Content: "x"}, {Role: "user", Content: "y"}, {Role: "user", Content: "z"}}} {
		if err := validateMessages(messages); err == nil {
			t.Error("accepted invalid messages")
		}
	}
	if endpoint("https://example.com/v1/chat/completions/") != "https://example.com/v1/chat/completions" {
		t.Fatal("endpoint duplicated")
	}
}
