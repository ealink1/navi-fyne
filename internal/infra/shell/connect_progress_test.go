package shell

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/ealink1/navi-fyne/internal/domain"
)

func TestSSHProgressMatchesRealOperations(t *testing.T) {
	for _, scenario := range []struct {
		name      string
		configure func(*sshFixture, *domain.ShellHost)
		failed    ConnectStage
		success   bool
	}{
		{name: "success", success: true},
		{name: "authentication rejected", configure: func(_ *sshFixture, host *domain.ShellHost) { host.Password = "rejected" }, failed: ConnectAuth},
		{name: "changed identity", configure: func(_ *sshFixture, host *domain.ShellHost) { host.Fingerprint = "SHA256:changed" }, failed: ConnectAuth},
		{name: "PTY rejected", configure: func(f *sshFixture, _ *domain.ShellHost) { f.rejectPTY.Store(true) }, failed: ConnectChannel},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newSSHFixture(t)
			host := fixture.host
			if scenario.configure != nil {
				scenario.configure(fixture, &host)
			}
			var events []ConnectEvent
			remote, err := OpenSSHWithProgress(context.Background(), host, func(event ConnectEvent) { events = append(events, event) })
			if (err == nil) != scenario.success {
				t.Fatalf("unexpected connection result: %v", err)
			}
			if remote != nil {
				remote.Close()
			}
			var expected []ConnectEvent
			for stage := ConnectTCP; stage <= ConnectReady; stage++ {
				expected = append(expected, ConnectEvent{Stage: stage, State: ConnectStarted})
				if !scenario.success && stage == scenario.failed {
					expected = append(expected, ConnectEvent{Stage: stage, State: ConnectFailed})
					break
				}
				expected = append(expected, ConnectEvent{Stage: stage, State: ConnectCompleted})
			}
			if !slices.Equal(events, expected) {
				t.Fatalf("events: %v; expected: %v", events, expected)
			}
		})
	}
}

func TestSSHProgressCancellationStopsOpeningChannel(t *testing.T) {
	fixture := newSSHFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var events []ConnectEvent
	remote, err := OpenSSHWithProgress(ctx, fixture.host, func(event ConnectEvent) {
		events = append(events, event)
		if event.Stage == ConnectChannel && event.State == ConnectStarted {
			cancel()
		}
	})
	if remote != nil {
		remote.Close()
	}
	if !errors.Is(err, context.Canceled) || remote != nil {
		t.Fatalf("channel startup ignored cancellation: %v", err)
	}
	last := events[len(events)-1]
	if last.State != ConnectFailed || slices.Contains(events, ConnectEvent{Stage: ConnectReady, State: ConnectCompleted}) {
		t.Fatalf("cancelled setup reported ready: %v", events)
	}
}
