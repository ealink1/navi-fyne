package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
	"github.com/ealink1/super-link/internal/upstream/connection"
	"strconv"
)

func (e *connectionEditor) networkForm(c connection.ConnectionConfig) fyne.CanvasObject {
	e.sshEnabled = widget.NewCheck("使用 SSH 隧道", nil)
	e.sshEnabled.SetChecked(c.UseSSH)
	e.sshHost = e.entry(c.SSH.Host, false)
	e.sshPort = e.entry(strconv.Itoa(max(c.SSH.Port, 22)), false)
	e.sshUser = e.entry(c.SSH.User, false)
	e.sshPassword = e.entry(c.SSH.Password, true)
	e.sshKey = e.entry(c.SSH.KeyPath, false)
	e.knownHosts = e.entry(c.SSH.KnownHostsPath, false)
	e.fingerprint = e.entry(c.SSH.HostKeyFingerprint, false)
	e.proxyEnabled = widget.NewCheck("使用网络代理", nil)
	e.proxyEnabled.SetChecked(c.UseProxy)
	e.proxyType = widget.NewSelect([]string{"socks5", "http"}, nil)
	e.proxyType.SetSelected(c.Proxy.Type)
	if e.proxyType.Selected == "" {
		e.proxyType.SetSelected("socks5")
	}
	e.proxyHost = e.entry(c.Proxy.Host, false)
	e.proxyPort = e.entry(strconv.Itoa(c.Proxy.Port), false)
	e.proxyUser = e.entry(c.Proxy.User, false)
	e.proxyPassword = e.entry(c.Proxy.Password, true)
	e.tlsEnabled = widget.NewCheck("使用 TLS", nil)
	e.tlsEnabled.SetChecked(c.UseSSL)
	e.tlsMode = widget.NewSelect([]string{"required", "preferred", "disable", "skip-verify"}, nil)
	e.tlsMode.SetSelected(c.SSLMode)
	if e.tlsMode.Selected == "" {
		e.tlsMode.SetSelected("required")
	}
	e.ca = e.entry(c.SSLCAPath, false)
	e.cert = e.entry(c.SSLCertPath, false)
	e.key = e.entry(c.SSLKeyPath, false)
	return widget.NewForm(widget.NewFormItem("SSH", e.sshEnabled), widget.NewFormItem("SSH 主机", e.sshHost), widget.NewFormItem("SSH 端口", e.sshPort), widget.NewFormItem("SSH 用户", e.sshUser), widget.NewFormItem("SSH 密码", e.sshPassword), widget.NewFormItem("私钥路径", e.sshKey), widget.NewFormItem("known_hosts 文件", e.knownHosts), widget.NewFormItem("服务器指纹", e.fingerprint), widget.NewFormItem("代理", e.proxyEnabled), widget.NewFormItem("代理类型", e.proxyType), widget.NewFormItem("代理主机", e.proxyHost), widget.NewFormItem("代理端口", e.proxyPort), widget.NewFormItem("代理用户", e.proxyUser), widget.NewFormItem("代理密码", e.proxyPassword), widget.NewFormItem("TLS", e.tlsEnabled), widget.NewFormItem("验证模式", e.tlsMode), widget.NewFormItem("CA 路径", e.ca), widget.NewFormItem("客户端证书", e.cert), widget.NewFormItem("客户端私钥", e.key))
}
