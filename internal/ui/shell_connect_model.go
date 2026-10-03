package ui

import (
	"sync"

	transport "github.com/ealink1/super-link/internal/infra/shell"
)

const shellConnectLogLimit = 64

type shellConnectModel struct {
	mu      sync.Mutex
	events  []transport.ConnectEvent
	changes chan struct{}
	closed  bool
}

func newShellConnectModel() *shellConnectModel {
	// One notification requests a snapshot of all changes; bursts never queue UI work.
	return &shellConnectModel{changes: make(chan struct{}, 1)}
}

func (m *shellConnectModel) record(event transport.ConnectEvent) {
	if event.Stage > transport.ConnectReady || event.State > transport.ConnectFailed {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || (len(m.events) > 0 && m.events[len(m.events)-1] == event) {
		return
	}
	if len(m.events) == shellConnectLogLimit {
		copy(m.events, m.events[1:])
		m.events = m.events[:shellConnectLogLimit-1]
	}
	m.events = append(m.events, event)
	select {
	case m.changes <- struct{}{}:
	default:
	}
}

func (m *shellConnectModel) snapshot() []transport.ConnectEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]transport.ConnectEvent(nil), m.events...)
}

func (m *shellConnectModel) close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.closed {
		m.closed = true
		close(m.changes)
	}
}

var shellConnectTitles = [...]string{"建立连接", "身份验证", "打开通道", "连接就绪"}

func shellConnectDescription(event transport.ConnectEvent) string {
	if event.State == transport.ConnectFailed {
		return "此阶段未完成，连接已停止"
	}
	descriptions := [...][2]string{
		{"正在建立 TCP 连接", "TCP 连接已建立"},
		{"正在核验主机身份并验证凭据", "SSH 身份验证已通过"},
		{"正在申请终端通道与交互式 Shell", "终端通道已打开"},
		{"正在准备终端会话", "SSH 连接就绪"},
	}
	return descriptions[event.Stage][event.State]
}
