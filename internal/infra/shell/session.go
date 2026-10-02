// Package shell owns terminal transports and remote operations without UI dependencies.
package shell

import "io"

// Session is a live PTY. Close must interrupt reads and release its process or connection.
type Session interface {
	io.ReadWriteCloser
	Resize(cols, rows int) error
}
