package shell

import (
	"context"
	"errors"

	"github.com/ealink1/navi-fyne/internal/domain"
)

// ConnectStage identifies an actual SSH setup operation.
type ConnectStage uint8

// SSH setup stages remain ordered for progress presentation.
const (
	ConnectTCP ConnectStage = iota
	ConnectAuth
	ConnectChannel
	ConnectReady
)

// ConnectState describes an operation's start, completion or failure.
type ConnectState uint8

// ConnectState values contain no server output or authentication material.
const (
	ConnectStarted ConnectState = iota
	ConnectCompleted
	ConnectFailed
)

// ConnectEvent deliberately carries only typed progress, never credentials or raw errors.
type ConnectEvent struct {
	Stage ConnectStage
	State ConnectState
}

// OpenSSHWithProgress reports real setup stages synchronously. The observer must
// return promptly; callers marshal these events onto their UI thread themselves.
func OpenSSHWithProgress(ctx context.Context, h domain.ShellHost, observe func(ConnectEvent)) (*Remote, error) {
	stage := ConnectTCP
	report := func(next ConnectStage, state ConnectState) {
		stage = next
		if observe != nil {
			observe(ConnectEvent{Stage: next, State: state})
		}
	}
	report(ConnectTCP, ConnectStarted)
	remote, err := openSSH(ctx, h, report)
	if err != nil {
		if ctx.Err() != nil {
			err = errors.Join(err, ctx.Err())
		}
		report(stage, ConnectFailed)
	}
	return remote, err
}
