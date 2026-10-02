//go:build !darwin && !linux

package shell

import (
	"context"
	"errors"
)

// OpenLocal reports platforms whose native PTY adapter is not yet implemented.
func OpenLocal(context.Context, string) (Session, error) {
	return nil, errors.New("此平台的本地 PTY 尚未实现；仍可连接 SSH 终端")
}
