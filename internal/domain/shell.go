package domain

import (
	"errors"
	"strings"
)

// ShellHost stores public connection metadata. Credentials are never serialized.
type ShellHost struct {
	ID, Name, Group, Host, User  string
	Port                         int
	KeyPath, Fingerprint         string
	Remember                     bool
	Revision                     int64
	SecretRef                    string `json:"-"`
	Password, Passphrase         string `json:"-"`
	Notes, Tags, OperatingSystem string
}

// Validate checks an SSH endpoint without interpreting host input as a command.
func (h ShellHost) Validate() error {
	if strings.TrimSpace(h.Name) == "" || len(h.Name) > 256 {
		return errors.New("主机名称不能为空，且最多 256 字节")
	}
	if strings.TrimSpace(h.Host) == "" || strings.ContainsAny(h.Host, "\x00\r\n/ ") || len(h.Host) > 253 {
		return errors.New("请输入有效的 SSH 主机地址")
	}
	if h.Port < 1 || h.Port > 65535 || strings.TrimSpace(h.User) == "" || len(h.User) > 256 || strings.ContainsAny(h.User, "\x00\r\n") {
		return errors.New("SSH 端口或用户名无效")
	}
	if len(h.Group) > 256 || len(h.Fingerprint) > 256 || len(h.KeyPath) > 4096 {
		return errors.New("主机分组或指纹过长")
	}
	if len(h.Notes) > 4096 || len(h.Tags) > 512 || len(h.OperatingSystem) > 64 {
		return errors.New("主机备注、标签或系统名称过长")
	}
	return nil
}
