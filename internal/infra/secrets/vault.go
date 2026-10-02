// Package secrets keeps credentials out of application metadata and logs.
package secrets

import (
	"errors"
	"strings"
	"sync"

	"github.com/google/uuid"
)

// ErrUnavailable indicates that a vault cannot access its local storage.
var ErrUnavailable = errors.New("无法访问本地密码存储，请检查工作区权限")

// ErrMissing indicates that credentials have not been retained or were removed.
var ErrMissing = errors.New("密码不可用，请编辑连接，重新输入密码并勾选“记住密码”")

// ErrLegacy requests explicit re-entry without touching the old system keychain.
var ErrLegacy = errors.New("此连接使用旧版钥匙串凭据，请重新输入一次密码并勾选“记住密码”；旧凭据仍保留在钥匙串中")

// Vault stores opaque credential bundles independently of public metadata.
type Vault interface {
	Put(data []byte, persistent bool) (string, error)
	Get(ref string) ([]byte, error)
	Delete(ref string) error
}

// System supports session credentials and optional local encrypted persistence.
// It never accesses an operating system keychain or prompts for a password.
type System struct {
	mu     sync.Mutex
	root   string
	memory map[string][]byte
}

// New creates a session-only vault, useful for isolated application tests.
func New() *System {
	return &System{memory: make(map[string][]byte)}
}

// NewLocal enables remembered credentials in the supplied application workspace.
// Files are created lazily, only when the user saves a nonempty credential bundle.
func NewLocal(root string) *System {
	v := New()
	v.root = root
	return v
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
	ref = localPrefix + ref
	if err := v.putLocal(ref, data); err != nil {
		return "", err
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
	if strings.HasPrefix(ref, localPrefix) {
		return v.getLocal(ref)
	}
	if validID(ref) {
		return nil, ErrLegacy
	}
	return nil, ErrMissing
}

func (v *System) Delete(ref string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if ref == "" || validID(ref) {
		// Old keychain entries are retained: removal would trigger OS authorization.
		return nil
	}
	if strings.HasPrefix(ref, "session:") {
		clear(v.memory[ref])
		delete(v.memory, ref)
		return nil
	}
	return v.deleteLocal(ref)
}
