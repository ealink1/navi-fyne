package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ealink1/super-link/internal/infra/chat"
)

func (p *aiPanel) send() {
	if p.owner.shuttingDown || p.loading || p.saving {
		return
	}
	if p.busy {
		if p.cancel != nil {
			p.cancel()
			p.status.SetText("正在停止回复…")
			p.sendButton.Disable()
		}
		return
	}
	if p.config.Model == "" {
		p.settings()
		return
	}
	text := strings.TrimSpace(p.input.Text)
	if text == "" {
		return
	}
	if !utf8.ValidString(text) || len(text) > chat.MaxInputBytes {
		p.status.SetText("问题最长 8 KiB，请缩短后发送")
		return
	}
	history, trimmed := aiRequestHistory(p.history, text)
	request := append(history, chat.Message{Role: "user", Content: text})
	p.busy = true
	p.generation++
	generation, config := p.generation, p.config
	if len(p.visibleText) == 0 {
		p.messages.Objects = nil
	}
	p.appendMessage("你", text)
	p.answer = p.appendMessage("SuperLink AI", "正在等待回复…")
	p.input.SetText("")
	p.status.SetText("正在连接 AI 服务…")
	p.refreshControls()
	// A single signal is sufficient: the callback reads the latest immutable
	// snapshot. Streaming cannot queue an unbounded number of UI redraws.
	signals := make(chan struct{}, 1)
	p.owner.jobs.watch(signals, func() {
		progress := p.progress.Load()
		if progress != nil && progress.generation == p.generation && p.busy {
			p.renderAnswer(progress.text, false)
			p.status.SetText("正在回复…")
		}
	})
	p.owner.jobs.runWithCancel(&p.cancel, func(ctx context.Context) (any, error) {
		defer close(signals)
		lastUpdate := time.Time{}
		return p.client.Reply(ctx, config, request, func(reply string) {
			if time.Since(lastUpdate) < 100*time.Millisecond {
				return
			}
			lastUpdate = time.Now()
			p.progress.Store(&aiProgress{generation: generation, text: reply})
			select {
			case signals <- struct{}{}:
			default:
			}
		})
	}, func(value any, err error) {
		if generation != p.generation {
			return
		}
		p.busy, p.cancel = false, nil
		p.refreshControls()
		reply, _ := value.(string)
		if err != nil {
			status := err.Error()
			if errors.Is(err, context.Canceled) {
				status = "回复已停止"
			}
			if reply == "" {
				reply = status
			}
			p.renderAnswer(reply, true)
			p.status.SetText(status + " · 本轮未加入后续上下文")
			// Do not replace text the user composed while waiting for a reply.
			if p.input.Text == "" {
				p.input.SetText(text)
			}
			return
		}
		p.history = append(request, chat.Message{Role: "assistant", Content: reply})
		p.renderAnswer(reply, true)
		status := fmt.Sprintf("回复完成 · %d 轮对话", len(p.history)/2)
		if trimmed {
			status += " · 较早的消息已移出上下文"
		}
		p.status.SetText(status)
	})
}

// Trim entire old turns, so requests always begin with a user message and never
// exceed the byte or message budget. The visible conversation stays independent.
func aiRequestHistory(history []chat.Message, input string) ([]chat.Message, bool) {
	size, start := len(input), 0
	for _, m := range history {
		size += len(m.Content)
	}
	for start+1 < len(history) && (len(history)-start+1 > chat.MaxMessages || size > chat.MaxContextBytes) {
		size -= len(history[start].Content) + len(history[start+1].Content)
		start += 2
	}
	return append([]chat.Message(nil), history[start:]...), start > 0
}
