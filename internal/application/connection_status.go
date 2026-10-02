package application

import "strings"

// ConnectionStatus describes an application's known session, without probing the server.
type ConnectionStatus uint32

const (
	// ConnectionDisconnected means no established session is available.
	ConnectionDisconnected ConnectionStatus = iota
	// ConnectionFailed means connection establishment failed or a session was discarded.
	ConnectionFailed
	// ConnectionConnecting means a driver is establishing a session.
	ConnectionConnecting
	// ConnectionConnected means a driver established a session that has not been discarded.
	ConnectionConnected
)

// ConnectionChanges coalesces state changes for the single application window.
// A receiver should re-read ConnectionStatuses rather than count notifications.
func (e *Engine) ConnectionChanges() <-chan struct{} { return e.connectionChanges }

// ConnectionStatuses returns an independent snapshot, keyed by profile ID.
// Multiple scope sessions are aggregated: an established session takes precedence.
func (e *Engine) ConnectionStatuses() map[string]ConnectionStatus {
	e.mu.Lock()
	defer e.mu.Unlock()
	states := make(map[string]ConnectionStatus)
	if e.closed {
		return states
	}
	for key, s := range e.sessions {
		id, _, _ := strings.Cut(key, "\x00")
		status := ConnectionStatus(s.connectionStatus.Load())
		states[id] = max(states[id], status)
	}
	return states
}

// Session state is atomic because snapshots must not wait for its I/O gate.
func (s *session) setConnectionStatus(status ConnectionStatus) {
	if s.connectionStatus.Swap(uint32(status)) == uint32(status) {
		return
	}
	select {
	case s.connectionChanges <- struct{}{}:
	default:
	}
}
