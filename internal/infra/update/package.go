// Package update stages and replaces complete application bundles.
package update

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/google/uuid"
)

const AppID = "io.github.ealink1.navifyne"
const Marker = "navi-fyne.package.json"

type Package struct {
	ID         string `json:"id"`
	Version    string `json:"version"`
	OS         string `json:"os"`
	Arch       string `json:"arch"`
	Executable string `json:"executable"`
	Helper     string `json:"helper"`
}
type Request struct {
	Target      string `json:"target"`
	Stage       string `json:"stage"`
	Backup      string `json:"backup"`
	ParentPID   int    `json:"parentPid"`
	Version     string `json:"version"`
	DataRoot    string `json:"dataRoot"`
	HealthFile  string `json:"healthFile"`
	Token       string `json:"token"`
	Report      string `json:"report"`
	StateBackup string `json:"stateBackup"`
}

func ReadPackage(root string) (Package, error) {
	var p Package
	info, err := os.Lstat(root)
	if err != nil {
		return p, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return p, errors.New("application root must be a real directory")
	}
	raw, err := os.ReadFile(filepath.Join(root, Marker))
	if errors.Is(err, os.ErrNotExist) && runtime.GOOS == "darwin" {
		raw, err = os.ReadFile(filepath.Join(root, "Contents", "Resources", Marker))
	}
	if err != nil {
		return p, err
	}
	if len(raw) > 4096 {
		return p, errors.New("package metadata too large")
	}
	if err = json.Unmarshal(raw, &p); err != nil {
		return p, err
	}
	if p.ID != AppID || p.Version == "" || p.OS != runtime.GOOS || p.Arch != runtime.GOARCH {
		return p, errors.New("package identity or platform mismatch")
	}
	for _, name := range []string{p.Executable, p.Helper} {
		if _, err = safeName(name); err != nil {
			return p, err
		}
		info, err = os.Lstat(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			return p, err
		}
		if !info.Mode().IsRegular() {
			return p, errors.New("package executable must be a regular file")
		}
	}
	return p, nil
}
func RunningRoot(executable string) (string, error) {
	root := filepath.Dir(executable)
	if runtime.GOOS == "darwin" {
		root = filepath.Clean(filepath.Join(root, "..", ".."))
	}
	p, err := ReadPackage(root)
	if err != nil {
		return "", errors.New("in-place updates require a packaged application; install the downloaded package manually for this development build")
	}
	expected, err := filepath.Abs(filepath.Join(root, filepath.FromSlash(p.Executable)))
	if err != nil {
		return "", err
	}
	actual, err := filepath.Abs(executable)
	if err != nil {
		return "", err
	}
	if expected != actual {
		return "", errors.New("running executable differs from package metadata")
	}
	return root, nil
}
func Prepare(ctx context.Context, archive, updates, target, version, dataRoot string) (Request, error) {
	var request Request
	if _, err := ReadPackage(target); err != nil {
		return request, err
	}
	extraction, err := os.MkdirTemp(updates, ".extract-*")
	if err != nil {
		return request, err
	}
	defer os.RemoveAll(extraction)
	if err = Extract(ctx, archive, extraction); err != nil {
		return request, err
	}
	entries, err := os.ReadDir(extraction)
	if err != nil || len(entries) != 1 || !entries[0].IsDir() {
		return request, errors.New("package archive must contain one application directory")
	}
	root := filepath.Join(extraction, entries[0].Name())
	p, err := ReadPackage(root)
	if err != nil {
		return request, err
	}
	if p.Version != version {
		return request, errors.New("package and signed manifest versions differ")
	}
	// The staging directory shares the target filesystem, so replacement is atomic.
	stage, err := os.MkdirTemp(filepath.Dir(target), ".navi-fyne-stage-*")
	if err != nil {
		return request, err
	}
	if err = copyTree(ctx, root, stage); err != nil {
		_ = os.RemoveAll(stage)
		return request, err
	}
	token := uuid.NewString()
	request = Request{Target: target, Stage: stage, Backup: target + ".backup-" + token, ParentPID: os.Getpid(), Version: version, DataRoot: dataRoot, HealthFile: filepath.Join(updates, "health-"+token+".json"), Token: token, Report: filepath.Join(updates, "last-update.json")}
	request.StateBackup = filepath.Join(updates, "state-"+token+".sqlite")
	return request, nil
}
func copyTree(ctx context.Context, source, target string) error {
	return filepath.WalkDir(source, func(name string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		relative, err := filepath.Rel(source, name)
		if err != nil {
			return err
		}
		destination := filepath.Join(target, relative)
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("application contains symlinks")
		}
		if entry.IsDir() {
			return os.MkdirAll(destination, 0700)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return errors.New("application contains a special file")
		}
		input, err := os.Open(name)
		if err != nil {
			return err
		}
		output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm()&0777)
		if err != nil {
			_ = input.Close()
			return err
		}
		_, copyErr := io.Copy(output, input)
		return errors.Join(copyErr, input.Close(), output.Sync(), output.Close())
	})
}
func ValidateRequest(r Request) error {
	for _, name := range []string{r.Target, r.Stage, r.Backup, r.DataRoot, r.HealthFile, r.Report, r.StateBackup} {
		if !filepath.IsAbs(name) || filepath.Clean(name) != name {
			return errors.New("update paths must be absolute and canonical")
		}
	}
	if r.ParentPID < 1 || r.Token == "" || strings.ContainsAny(r.Token, "/\\\x00") {
		return errors.New("invalid update identity")
	}
	if r.StateBackup != filepath.Join(filepath.Dir(r.HealthFile), "state-"+r.Token+".sqlite") {
		return errors.New("invalid state backup path")
	}
	if _, err := os.Lstat(r.StateBackup); !errors.Is(err, os.ErrNotExist) {
		return errors.New("state backup already exists")
	}
	if filepath.Dir(r.Target) != filepath.Dir(r.Stage) || filepath.Dir(r.Target) != filepath.Dir(r.Backup) || r.Target == r.Stage || r.Target == r.Backup {
		return errors.New("update staging and backup must be distinct siblings")
	}
	if !strings.HasPrefix(filepath.Base(r.Stage), ".navi-fyne-stage-") || r.Backup != r.Target+".backup-"+r.Token {
		return errors.New("invalid update staging or backup path")
	}
	if _, err := os.Lstat(r.Backup); !errors.Is(err, os.ErrNotExist) {
		return errors.New("update backup path already exists")
	}
	if _, err := ReadPackage(r.Target); err != nil {
		return err
	}
	p, err := ReadPackage(r.Stage)
	if err != nil {
		return err
	}
	if p.Version != r.Version {
		return errors.New("staged version mismatch")
	}
	return nil
}
