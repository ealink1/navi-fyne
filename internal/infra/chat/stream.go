package chat

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

// SSE parsing bounds wire bytes, individual events and final UTF-8 text. Only
// the first choice is displayed. An abruptly closed stream is not a success.
func readStream(ctx context.Context, reader io.Reader, onText func(string)) (string, error) {
	limited := &io.LimitedReader{R: reader, N: maxWireBytes + 1}
	scanner := bufio.NewScanner(limited)
	scanner.Buffer(make([]byte, 4096), 128<<10)
	var event, text strings.Builder
	finished := false
	consume := func() (bool, error) {
		data := strings.TrimSpace(event.String())
		event.Reset()
		if data == "" {
			return false, nil
		}
		if data == "[DONE]" {
			return true, nil
		}
		var result completion
		if err := json.Unmarshal([]byte(data), &result); err != nil || len(result.Error) > 0 && string(result.Error) != "null" {
			return false, errors.New("AI 流式响应无效，请检查接口兼容性")
		}
		for _, choice := range result.Choices {
			if choice.Index != 0 {
				continue
			}
			if text.Len()+len(choice.Delta.Content) > MaxReplyBytes {
				return false, errors.New("AI 回复超过 32 KiB 限制")
			}
			text.WriteString(choice.Delta.Content)
			if choice.Delta.Content != "" && onText != nil {
				onText(text.String())
			}
			if choice.Finish != nil && *choice.Finish != "" {
				finished = true
			}
		}
		return false, nil
	}
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return text.String(), err
		}
		if limited.N <= 0 {
			return text.String(), errors.New("AI 流式响应超过读取限制")
		}
		line := scanner.Text()
		if line == "" {
			done, err := consume()
			if err != nil {
				return text.String(), err
			}
			if done {
				finished = true
				break
			}
		} else if data, ok := strings.CutPrefix(line, "data:"); ok {
			if event.Len()+len(data) > 128<<10 {
				return text.String(), errors.New("AI 流式事件超过读取限制")
			}
			event.WriteString(strings.TrimPrefix(data, " "))
			event.WriteByte('\n')
		}
	}
	if err := ctx.Err(); err != nil {
		return text.String(), err
	}
	if scanner.Err() != nil {
		return text.String(), responseReadError(ctx)
	}
	if event.Len() > 0 {
		done, err := consume()
		if err != nil {
			return text.String(), err
		}
		finished = finished || done
	}
	if !finished {
		return text.String(), errors.New("AI 回复中途断开，请重试")
	}
	if strings.TrimSpace(text.String()) == "" {
		return "", errors.New("AI 没有返回文字回复，请检查模型是否支持普通问答")
	}
	return text.String(), nil
}
