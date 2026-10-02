package shell

import (
	"context"
	"golang.org/x/crypto/ssh"
)

// auxiliarySession uses transport cancellation only while opening a channel.
// Once open, cancellation closes that channel and leaves the terminal intact.
func (r *Remote) auxiliarySession(ctx context.Context) (*ssh.Session, func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	stopOpening := context.AfterFunc(ctx, func() { r.client.Close() })
	session, err := r.client.NewSession()
	stopOpening()
	if err != nil {
		return nil, nil, err
	}
	if err := ctx.Err(); err != nil {
		session.Close()
		return nil, nil, err
	}
	stop := context.AfterFunc(ctx, func() { session.Close() })
	return session, func() { stop(); session.Close() }, nil
}
