package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"github.com/ealink1/navi-fyne/internal/infra/chat"
)

func aiTestPanel(t *testing.T, w *Window) *aiPanel {
	t.Helper()
	w.aiEntry()
	p := w.switcher.ai
	pumpShell(t, w, func() bool { return !p.loading })
	if p == nil || !w.switcher.aiHost.Visible() {
		t.Fatal("AI panel not displayed")
	}
	return p
}

func TestAIChatMultiTurnAndWorkspaceIsolation(t *testing.T) {
	w, note := noteTestWindow(t)
	note.newNote()
	note.editor.SetText("private note must not leave workspace")
	p := aiTestPanel(t, w)
	var mu sync.Mutex
	var requests [][]chat.Message
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []chat.Message `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		mu.Lock()
		requests = append(requests, body.Messages)
		mu.Unlock()
		fmt.Fprint(rw, `{"choices":[{"message":{"content":"**测试回答**\n\n继续提问即可。"}}]}`)
	}))
	defer server.Close()
	p.config = chat.Config{BaseURL: server.URL + "/v1", Model: "fixture"}
	p.input.SetText("第一问")
	p.input.TypedShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyReturn, Modifier: fyne.KeyModifierSuper})
	pumpShell(t, w, func() bool { return !p.busy })
	if len(p.history) != 2 || len(p.messages.Objects) != 2 || p.history[1].Content != "**测试回答**\n\n继续提问即可。" {
		t.Fatal("first turn failed", p.status.Text)
	}
	p.input.SetText("第二问")
	w.switcher.selectMode(0)
	w.switcher.selectMode(1)
	w.switcher.selectMode(2)
	if w.switcher.ai != p || p.input.Text != "第二问" || note.editor.Text != "private note must not leave workspace" || !w.switcher.aiHost.Visible() {
		t.Fatal("workspace switch lost state")
	}
	w.aiEntry()
	if w.switcher.aiHost.Visible() {
		t.Fatal("close did not hide sidebar")
	}
	w.aiEntry()
	p.send()
	pumpShell(t, w, func() bool { return !p.busy })
	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 2 || len(requests[1]) != 3 || requests[1][2].Content != "第二问" {
		t.Fatal("multi-turn context missing")
	}
	for _, request := range requests {
		for _, message := range request {
			if strings.Contains(message.Content, "private note") {
				t.Fatal("workspace context leaked")
			}
		}
	}
	w.setDark(true)
	if !p.content.Theme.(shellTheme).palette.dark.Load() {
		t.Fatal("AI theme not shared")
	}
	w.Window.Resize(fyne.NewSize(960, 640))
	w.switcher.workspaceHost.Refresh()
	if p.content.Size().Width <= 0 || w.switcher.aiHost.Position().X <= w.switcher.body.Position().X {
		t.Fatal("AI not on right")
	}
	p.newConversation()
	if len(p.history) != 0 || p.input.Text != "" || p.answer != nil {
		t.Fatal("new chat retained context")
	}
}

func TestAIStopAndClearIgnoreLateStreamingCallbacks(t *testing.T) {
	w := shellTestWindow(t)
	p := aiTestPanel(t, w)
	stopped := make(chan struct{}, 2)
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(rw, "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n")
		rw.(http.Flusher).Flush()
		<-r.Context().Done()
		stopped <- struct{}{}
	}))
	defer server.Close()
	p.config = chat.Config{BaseURL: server.URL + "/v1", Model: "fixture", Stream: true}
	p.input.SetText("cancelled question")
	p.send()
	pumpShell(t, w, func() bool { return p.answer.String() == "partial" })
	p.input.SetText("typed while waiting")
	p.send() // stop button
	pumpShell(t, w, func() bool { return !p.busy })
	if len(p.history) != 0 || p.input.Text != "typed while waiting" || !strings.Contains(p.status.Text, "停止") {
		t.Fatal("cancel changed completed context/input")
	}
	p.input.SetText("clear while streaming")
	p.send()
	pumpShell(t, w, func() bool { return p.answer.String() == "partial" })
	p.newConversation()
	pumpShell(t, w, func() bool { return w.jobs.workers.Load() == 0 })
	waitUI(t, w)
	if p.answer != nil || len(p.visibleText) != 0 || len(p.history) != 0 || p.busy {
		t.Fatal("late reply revived cleared chat")
	}
	for range 2 {
		select {
		case <-stopped:
		case <-time.After(2 * time.Second):
			t.Fatal("HTTP request leaked")
		}
	}
}

func TestAIHistoryAndVisibleMemoryBounds(t *testing.T) {
	history := make([]chat.Message, 0, 40)
	for range 20 {
		history = append(history, chat.Message{Role: "user", Content: "q"}, chat.Message{Role: "assistant", Content: strings.Repeat("x", chat.MaxReplyBytes)})
	}
	request, trimmed := aiRequestHistory(history, "next")
	size := len("next")
	for _, message := range request {
		size += len(message.Content)
	}
	if !trimmed || size > chat.MaxContextBytes || len(request)%2 != 0 || request[0].Role != "user" {
		t.Fatal("context budget failed")
	}
	w := shellTestWindow(t)
	p := aiTestPanel(t, w)
	p.messages.Objects = nil
	for range 20 {
		p.appendMessage("你", "q")
		p.answer = p.appendMessage("AI", strings.Repeat("x", chat.MaxReplyBytes))
	}
	size = 0
	for _, text := range p.visibleText {
		size += len(text)
	}
	if size > 128<<10 || len(p.messages.Objects) != len(p.visibleText) {
		t.Fatal("display memory unbounded")
	}
	p.newConversation()
	p.config = chat.Config{BaseURL: "http://localhost:1/v1", Model: "fixture"}
	p.input.SetText(strings.Repeat("x", chat.MaxInputBytes+1))
	p.send()
	if p.busy || len(p.history) != 0 || !strings.Contains(p.status.Text, "8 KiB") {
		t.Fatal("oversized input sent")
	}
	w.shuttingDown = true
	w.aiEntry()
	p.send()
	if p.busy {
		t.Fatal("shutdown allowed request")
	}
	w.shuttingDown = false
}

func TestAIConfigurationPersistenceDoesNotSendRequest(t *testing.T) {
	w, _ := noteTestWindow(t)
	p := aiTestPanel(t, w)
	config := chat.Config{BaseURL: "http://localhost:1/v1", Model: "fixture", APIKey: "fixture-private-key", Stream: true}
	if _, err := p.service.Save(context.Background(), config); err != nil {
		t.Fatal(err)
	}
	loaded, err := p.service.Load(context.Background())
	if err != nil || loaded != config || p.busy {
		t.Fatal("configuration save sent a request", err)
	}
}

func TestAIRecoveryAfterCancelledShutdownReconcilesSettings(t *testing.T) {
	w, _ := noteTestWindow(t)
	p := aiTestPanel(t, w)
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(rw, "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n")
		rw.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	config := chat.Config{BaseURL: server.URL + "/v1", Model: "fixture", Stream: true}
	if _, err := p.service.Save(context.Background(), config); err != nil {
		t.Fatal(err)
	}
	p.config = config
	p.input.SetText("shutdown test")
	p.send()
	pumpShell(t, w, func() bool { return p.answer.String() == "partial" })
	p.input.SetText("retain unsent input")
	w.switcher.stop()
	w.jobs.stop()
	w.jobs = newTasks(w.dispatch)
	p.saving = true // A save completion can be suppressed by task shutdown.
	w.restartAI()
	pumpShell(t, w, func() bool { return !p.loading })
	if p.busy || p.saving || p.config != config || p.sendButton.Disabled() || p.input.Text != "retain unsent input" {
		t.Fatal("failed close left chat stuck", p.status.Text)
	}
}
