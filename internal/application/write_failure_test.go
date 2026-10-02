package application

import (
	"errors"
	"testing"

	"github.com/ealink1/navi-fyne/internal/domain"
)

func TestConfirmedRollbackKeepsMemoryDatabaseSessionAndUnknownOutcomeClosesIt(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		closed bool
	}{
		{"confirmed", &domain.RolledBackError{Cause: errors.New("unique constraint")}, false},
		{"unknown", domain.ErrUnknownOutcome, true},
		{"transport", errors.New("lost transport"), true},
		{"unknown outranks rollback", &domain.RolledBackError{Cause: domain.ErrUnknownOutcome}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &fakeClient{}
			s := &session{client: client}
			discardFailedWriteSession(s, tc.err)
			if (s.client == nil) != tc.closed || (client.closed.Load() == 1) != tc.closed {
				t.Fatal("incorrect session lifetime after write failure")
			}
		})
	}
}
