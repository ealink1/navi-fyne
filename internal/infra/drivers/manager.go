package drivers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/ealink1/super-link/internal/domain"
	"github.com/ealink1/super-link/internal/infra/release"
	"github.com/ealink1/super-link/internal/upstream/appdata"
	"github.com/ealink1/super-link/internal/upstream/db"
)

type Manager struct {
	Root string
	once sync.Once
	gate chan struct{}
}
type Record struct {
	Type     string `json:"type"`
	SHA256   string `json:"sha256"`
	Revision string `json:"revision"`
	Protocol string `json:"protocol"`
	Source   string `json:"source"`
}
type Bundle struct {
	Schema  int      `json:"schema"`
	OS      string   `json:"os"`
	Arch    string   `json:"arch"`
	Drivers []Record `json:"drivers"`
}
type Status struct {
	Descriptor domain.Descriptor
	Installed  bool
	Message    string
}

func (m *Manager) Statuses(ctx context.Context) []Status {
	result := []Status{}
	for _, d := range domain.Catalog() {
		if ctx.Err() != nil {
			break
		}
		if !d.Agent {
			continue
		}
		err := m.Verify(ctx, d.Key)
		message := "已安装，摘要和兼容版本匹配"
		if err != nil {
			message = err.Error()
		}
		result = append(result, Status{Descriptor: d, Installed: err == nil, Message: message})
	}
	return result
}
func (m *Manager) Verify(ctx context.Context, kind string) error {
	unlock, err := m.lock(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	return m.verify(ctx, kind)
}
func (m *Manager) lock(ctx context.Context) (func(), error) {
	m.once.Do(func() { m.gate = make(chan struct{}, 1) })
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case m.gate <- struct{}{}:
		return func() { <-m.gate }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func (m *Manager) RunVerified(ctx context.Context, kind string, fn func() error) error {
	if d, err := domain.Resolve(kind); err == nil && !d.Agent {
		return fn()
	}
	unlock, err := m.lock(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	if err := m.verify(ctx, kind); err != nil {
		return err
	}
	return fn()
}
func (m *Manager) verify(ctx context.Context, kind string) error {
	descriptor, err := domain.Resolve(kind)
	if err != nil {
		return err
	}
	if !descriptor.Agent {
		return nil
	}
	executable, err := db.ResolveOptionalDriverAgentExecutablePath(m.Root, descriptor.Key)
	if err != nil {
		return err
	}
	marker, err := db.ResolveOptionalGoDriverMarkerPath(m.Root, descriptor.Key)
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(marker)
	if err != nil {
		return fmt.Errorf("%s 驱动尚未安装", descriptor.Name)
	}
	var record Record
	if err = json.Unmarshal(raw, &record); err != nil {
		return err
	}
	if record.Type != descriptor.Key || record.Revision != db.OptionalDriverAgentRevision(descriptor.Key) || record.Protocol != db.OptionalDriverAgentProtocolSchemaV2 {
		return errors.New("driver compatibility metadata mismatch; reinstall it")
	}
	digest, err := fileHash(ctx, executable)
	if err != nil {
		return err
	}
	if digest != record.SHA256 {
		return errors.New("driver binary checksum mismatch; reinstall it")
	}
	if err = validateArchitecture(executable); err != nil {
		return err
	}
	return db.ValidateOptionalDriverAgentExecutable(descriptor.Key, executable)
}
func fileHash(ctx context.Context, path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	buffer := make([]byte, 1<<20)
	for {
		if err = ctx.Err(); err != nil {
			return "", err
		}
		n, readErr := file.Read(buffer)
		if n > 0 {
			_, _ = hash.Write(buffer[:n])
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return "", readErr
		}
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
func (m *Manager) Install(ctx context.Context, kind, source, expectedHash, origin string) error {
	unlock, err := m.lock(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	descriptor, err := domain.Resolve(kind)
	if err != nil {
		return err
	}
	if !descriptor.Agent {
		return errors.New("not an optional driver")
	}
	digest, err := fileHash(ctx, source)
	if err != nil {
		return err
	}
	if !strings.EqualFold(digest, expectedHash) {
		return errors.New("driver checksum mismatch")
	}
	target, err := db.ResolveOptionalDriverAgentExecutablePath(m.Root, descriptor.Key)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return err
	}
	staged, err := os.CreateTemp(filepath.Dir(target), ".agent-*")
	if err != nil {
		return err
	}
	stagedPath := staged.Name()
	defer os.Remove(stagedPath)
	input, err := os.Open(source)
	if err != nil {
		_ = staged.Close()
		return err
	}
	_, copyErr := io.Copy(staged, input)
	err = errors.Join(copyErr, input.Close(), staged.Sync(), staged.Close())
	if err != nil {
		return err
	}
	if err = os.Chmod(stagedPath, 0700); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	stagedHash, err := fileHash(ctx, stagedPath)
	if err != nil {
		return err
	}
	if stagedHash != digest {
		return errors.New("driver changed while copying")
	}
	if err = validateArchitecture(stagedPath); err != nil {
		return err
	}
	if err = db.ValidateOptionalDriverAgentExecutable(kind, stagedPath); err != nil {
		return err
	}
	metadata, err := db.ProbeOptionalDriverAgentMetadata(kind, stagedPath)
	if err != nil {
		return err
	}
	if metadata.DriverType != descriptor.Key || metadata.AgentRevision != db.OptionalDriverAgentRevision(kind) || metadata.ProtocolSchema != db.OptionalDriverAgentProtocolSchemaV2 {
		return errors.New("driver handshake is incompatible")
	}
	record := Record{Type: descriptor.Key, SHA256: digest, Revision: metadata.AgentRevision, Protocol: metadata.ProtocolSchema, Source: origin}
	// Atomic file replacement leaves existing running processes untouched. Their
	// sessions are disconnected by the application before calling Install.
	if err = appdata.AtomicReplaceFile(stagedPath, target); err != nil {
		return err
	}
	raw, err := json.Marshal(record)
	if err != nil {
		return err
	}
	marker, _ := db.ResolveOptionalGoDriverMarkerPath(m.Root, kind)
	return writeAtomic(marker, raw)
}
func writeAtomic(target string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(target), ".metadata-*")
	if err != nil {
		return err
	}
	path := file.Name()
	defer os.Remove(path)
	_, writeErr := file.Write(data)
	err = errors.Join(writeErr, file.Sync(), file.Close())
	if err != nil {
		return err
	}
	return appdata.AtomicReplaceFile(path, target)
}
func (m *Manager) InstallBundled(ctx context.Context, directory string) error {
	raw, err := os.ReadFile(filepath.Join(directory, "bundle.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var bundle Bundle
	if err = json.Unmarshal(raw, &bundle); err != nil {
		return err
	}
	if bundle.Schema != 1 || bundle.OS != runtime.GOOS || bundle.Arch != runtime.GOARCH {
		return errors.New("bundled drivers target another platform")
	}
	for _, record := range bundle.Drivers {
		if err = ctx.Err(); err != nil {
			return err
		}
		if verifyErr := m.Verify(ctx, record.Type); verifyErr == nil {
			continue
		}
		name := record.Type + "-driver-agent"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		descriptor, resolveErr := domain.Resolve(record.Type)
		if resolveErr != nil || !descriptor.Agent {
			return errors.New("invalid bundled driver type")
		}
		if err = m.Install(ctx, record.Type, filepath.Join(directory, name), record.SHA256, "application-bundle"); err != nil {
			return err
		}
	}
	return nil
}
func (m *Manager) InstallRelease(ctx context.Context, c *release.Client, manifest release.Manifest, kind, staging string) error {
	artifact, err := manifest.Artifact("driver", kind, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return err
	}
	if artifact.Revision != db.OptionalDriverAgentRevision(kind) || artifact.Protocol != db.OptionalDriverAgentProtocolSchemaV2 {
		return errors.New("release driver is incompatible with this application")
	}
	file, err := c.Download(ctx, artifact, staging)
	if err != nil {
		return err
	}
	defer os.Remove(file)
	return m.Install(ctx, kind, file, artifact.SHA256, "signed-release:"+manifest.Version)
}
