// Package bootstrap owns application lifetime and native services. It has no UI.
package bootstrap

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/ealink1/navi-fyne/internal/application"
	"github.com/ealink1/navi-fyne/internal/domain"
	"github.com/ealink1/navi-fyne/internal/infra/drivers"
	"github.com/ealink1/navi-fyne/internal/infra/instance"
	"github.com/ealink1/navi-fyne/internal/infra/release"
	adapter "github.com/ealink1/navi-fyne/internal/infra/runtime"
	"github.com/ealink1/navi-fyne/internal/infra/secrets"
	"github.com/ealink1/navi-fyne/internal/infra/state"
	"github.com/ealink1/navi-fyne/internal/upstream/appdata"
	"github.com/ealink1/navi-fyne/internal/upstream/db"
	"github.com/ealink1/navi-fyne/internal/upstream/logger"
	"github.com/ealink1/navi-fyne/internal/upstream/proxy"
	"github.com/ealink1/navi-fyne/internal/upstream/ssh"
)

type Services struct {
	Root     string
	Store    *state.Store
	Profiles *application.Profiles
	Engine   *application.Engine
	Drivers  *drivers.Manager
	Releases *release.Client
	lock     *instance.Lock
}

func Open(root, bundle, key string) (*Services, error) {
	if root == "" {
		root = os.Getenv("NAVIFYNE_DATA_ROOT")
	}
	if root == "" {
		directory, err := os.UserConfigDir()
		if err != nil {
			return nil, err
		}
		root = filepath.Join(directory, "NaviFyne")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	lock, err := instance.Acquire(root)
	if err != nil {
		return nil, err
	}
	opened := false
	defer func() {
		if !opened {
			_ = lock.Close()
		}
	}()
	if _, err = appdata.SetActiveRoot(root); err != nil {
		return nil, err
	}
	store, err := state.Open(filepath.Join(root, "state.sqlite"))
	if err != nil {
		return nil, err
	}
	s := &Services{Root: root, Store: store, Drivers: &drivers.Manager{Root: filepath.Join(root, "drivers")}, Releases: release.New(key)}
	s.lock = lock
	s.Profiles = &application.Profiles{Store: store, Vault: secrets.New()}
	s.Engine = application.NewEngine(s.Profiles)
	db.SetExternalDriverDownloadDirectory(s.Drivers.Root)
	s.Engine.Factory = func(ctx context.Context, p domain.Profile) (adapter.Client, error) {
		var client adapter.Client
		err := s.Drivers.RunVerified(ctx, p.Config.Type, func() error { var err error; client, err = adapter.Open(ctx, p); return err })
		return client, err
	}
	if bundle == "" {
		executable, _ := os.Executable()
		bundle = filepath.Join(filepath.Dir(executable), "drivers")
		if _, err = os.Stat(bundle); err != nil {
			bundle = filepath.Join(filepath.Dir(executable), "..", "Resources", "drivers")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err = s.Drivers.InstallBundled(ctx, bundle); err != nil {
		_ = s.Close()
		return nil, err
	}
	opened = true
	return s, nil
}
func (s *Services) Close() error {
	err := s.Engine.Close()
	ssh.CloseAllForwarders()
	proxy.CloseAllForwarders()
	logger.Close()
	return errors.Join(err, s.Store.Close(), s.lock.Close())
}
