package application

import (
	"errors"

	"github.com/ealink1/super-link/internal/domain"
)

func discardFailedWriteSession(s *session, err error) {
	if err == nil {
		return
	}
	var rolledBack *domain.RolledBackError
	if errors.As(err, &rolledBack) && !errors.Is(err, domain.ErrUnknownOutcome) {
		return
	}
	_ = s.client.Close()
	s.client = nil
	s.setConnectionStatus(ConnectionFailed)
}
