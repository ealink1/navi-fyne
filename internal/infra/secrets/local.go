package secrets

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

const (
	localPrefix   = "local:v1:"
	localHeader   = "NFCR\x01"
	keyName       = "master.key"
	maxBundleSize = 8 << 20
)

// ErrCorrupt indicates that a credential bundle or its key cannot be trusted.
var ErrCorrupt = errors.New("本地密码文件或密钥损坏，请恢复完整的 credentials 备份；无备份时需重置本地密码存储再重新输入")

func validID(id string) bool {
	u, err := uuid.Parse(id)
	return err == nil && u.String() == id
}

func localName(ref string) (string, error) {
	id, ok := strings.CutPrefix(ref, localPrefix)
	if !ok || !validID(id) {
		return "", ErrMissing
	}
	return id + ".cred", nil
}

func (v *System) localDirectory(create bool) (string, error) {
	if v.root == "" {
		return "", ErrUnavailable
	}
	dir := filepath.Join(v.root, "credentials")
	if create {
		err := os.Mkdir(dir, 0700)
		if err != nil && !errors.Is(err, os.ErrExist) {
			return "", ErrUnavailable
		}
		if err == nil {
			if err = syncDirectory(v.root); err != nil {
				return "", ErrUnavailable
			}
		}
	}
	info, err := os.Lstat(dir)
	if errors.Is(err, os.ErrNotExist) {
		return "", ErrMissing
	}
	if err != nil || !info.IsDir() || !privatePermissions(info) {
		return "", ErrUnavailable
	}
	return dir, nil
}

func localCipher(dir string, create bool) (cipher.AEAD, error) {
	key, err := readPrivate(filepath.Join(dir, keyName), 32)
	if errors.Is(err, os.ErrNotExist) && create {
		entries, listErr := os.ReadDir(dir)
		if listErr != nil {
			return nil, ErrUnavailable
		}
		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), ".cred") {
				// Never replace a lost key while ciphertext still needs recovery.
				// A concurrent creator may have published the key since our read.
				return localCipher(dir, false)
			}
		}
		key = make([]byte, 32)
		if _, err = rand.Read(key); err != nil {
			clear(key)
			return nil, ErrUnavailable
		}
		err = writePrivate(dir, keyName, key)
		clear(key)
		if err != nil && !errors.Is(err, os.ErrExist) {
			return nil, ErrUnavailable
		}
		key, err = readPrivate(filepath.Join(dir, keyName), 32)
	}
	if err != nil || len(key) != 32 {
		clear(key)
		return nil, ErrCorrupt
	}
	defer clear(key)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, ErrCorrupt
	}
	return cipher.NewGCMWithRandomNonce(block)
}

func (v *System) putLocal(ref string, data []byte) error {
	if len(data) > maxBundleSize {
		return errors.New("密码和敏感连接参数合计不能超过 8 MiB")
	}
	name, err := localName(ref)
	if err != nil {
		return err
	}
	dir, err := v.localDirectory(true)
	if err != nil {
		return err
	}
	aead, err := localCipher(dir, true)
	if err != nil {
		return err
	}
	encrypted := aead.Seal([]byte(localHeader), nil, data, []byte(ref))
	if err = writePrivate(dir, name, encrypted); err != nil {
		return ErrUnavailable
	}
	return nil
}

func (v *System) getLocal(ref string) ([]byte, error) {
	name, err := localName(ref)
	if err != nil {
		return nil, err
	}
	dir, err := v.localDirectory(false)
	if err != nil {
		return nil, err
	}
	encrypted, err := readPrivate(filepath.Join(dir, name), maxBundleSize+64)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrMissing
	}
	if err != nil || !bytes.HasPrefix(encrypted, []byte(localHeader)) {
		return nil, ErrCorrupt
	}
	aead, err := localCipher(dir, false)
	if err != nil {
		return nil, err
	}
	data, err := aead.Open(nil, nil, encrypted[len(localHeader):], []byte(ref))
	if err != nil {
		return nil, ErrCorrupt
	}
	return data, nil
}

func (v *System) deleteLocal(ref string) error {
	name, err := localName(ref)
	if err != nil {
		return err
	}
	dir, err := v.localDirectory(false)
	if errors.Is(err, ErrMissing) {
		return nil
	}
	if err != nil {
		return err
	}
	if err = os.Remove(filepath.Join(dir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return ErrUnavailable
	}
	return nil
}
