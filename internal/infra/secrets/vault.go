// Package secrets keeps credentials out of application metadata and logs.
package secrets

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/99designs/keyring"
	"github.com/google/uuid"
)

var ErrUnavailable = errors.New("system credential store unavailable; choose session-only credentials")
var ErrMissing = errors.New("credentials unavailable; edit the connection and enter them again")

type Vault interface {
	Put(data []byte, persistent bool) (string, error)
	Get(ref string) ([]byte, error)
	Delete(ref string) error
}

type System struct {
	mu     sync.Mutex
	ring   keyring.Keyring
	memory map[string][]byte
}

func New() *System {
	return &System{memory: make(map[string][]byte)}
}

// Opening the native keychain is lazy: startup must not prompt for a password.
func (v *System) open() error {
	if v.ring != nil {
		return nil
	}
	ring, err := keyring.Open(keyring.Config{
		ServiceName: "io.github.ealink1.navifyne", WinCredPrefix: "NaviFyne",
		AllowedBackends:                []keyring.BackendType{keyring.KeychainBackend, keyring.WinCredBackend, keyring.SecretServiceBackend, keyring.KWalletBackend},
		KeychainAccessibleWhenUnlocked: true,
	})
	if err != nil {
		return ErrUnavailable
	}
	v.ring = ring
	return nil
}

func (v *System) Put(data []byte, persistent bool) (string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if len(data) == 0 || string(data) == "{}" {
		return "", nil
	}
	ref := uuid.NewString()
	if !persistent {
		ref = "session:" + ref
		v.memory[ref] = append([]byte(nil), data...)
		return ref, nil
	}
	if err := v.open(); err != nil {
		return "", err
	}
	if err := v.ring.Set(keyring.Item{Key: ref, Data: append([]byte(nil), data...), Label: "Navi Fyne connection credentials"}); err != nil {
		return "", fmt.Errorf("%w", ErrUnavailable)
	}
	return ref, nil
}

func (v *System) Get(ref string) ([]byte, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if ref == "" {
		return []byte("{}"), nil
	}
	if strings.HasPrefix(ref, "session:") {
		data, ok := v.memory[ref]
		if !ok {
			return nil, ErrMissing
		}
		return append([]byte(nil), data...), nil
	}
	if err := v.open(); err != nil {
		return nil, err
	}
	item, err := v.ring.Get(ref)
	if err != nil {
		return nil, ErrMissing
	}
	return append([]byte(nil), item.Data...), nil
}
func (v *System) Delete(ref string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if ref == "" {
		return nil
	}
	if strings.HasPrefix(ref, "session:") {
		if data := v.memory[ref]; data != nil {
			clear(data)
		}
		delete(v.memory, ref)
		return nil
	}
	if err := v.open(); err != nil {
		return err
	}
	err := v.ring.Remove(ref)
	if errors.Is(err, keyring.ErrKeyNotFound) {
		return nil
	}
	return err
}
