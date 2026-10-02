package ui

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/ealink1/navi-fyne/internal/domain"
	"golang.org/x/crypto/ssh"
)

func TestSSHConfirmationUsesConnectedEndpointInsteadOfStaleList(t *testing.T) {
	w := shellTestWindow(t)
	w.switcher.selectMode(1)
	waitUI(t, w)
	s := w.switcher.shell
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	config := &ssh.ServerConfig{NoClientAuth: true}
	config.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		connection, err := listener.Accept()
		if err != nil {
			return
		}
		defer connection.Close()
		connection.SetDeadline(time.Now().Add(5 * time.Second))
		_, _, _, _ = ssh.NewServerConn(connection, config)
	}()
	_, port, _ := net.SplitHostPort(listener.Addr().String())
	number, _ := strconv.Atoi(port)
	h, err := s.service.Save(context.Background(), domain.ShellHost{Name: "fixture", Host: "127.0.0.1", Port: number, User: "old-user"})
	if err != nil {
		t.Fatal(err)
	}
	current := h
	current.Host = "localhost"
	current.User = "current-user"
	if _, err := s.service.Save(context.Background(), current); err != nil {
		t.Fatal(err)
	}
	s.connectHost(h)
	pane := s.panes[s.tabs.Selected()]
	pumpShell(t, w, func() bool { return pane.ended })
	text := ""
	for _, overlay := range w.Window.Canvas().Overlays().List() {
		text += shellDialogText(overlay)
	}
	if !strings.Contains(text, "current-user@localhost:") || strings.Contains(text, "old-user@127.0.0.1:") || !strings.Contains(text, ssh.FingerprintSHA256(signer.PublicKey())) {
		t.Fatal("confirmation does not describe negotiated endpoint", text)
	}
	select {
	case <-serverDone:
	case <-time.After(time.Second):
		t.Fatal("failed handshake leaked server connection")
	}
}

func shellDialogText(object fyne.CanvasObject) string {
	switch object := object.(type) {
	case *widget.Label:
		return object.Text
	case *fyne.Container:
		var text strings.Builder
		for _, child := range object.Objects {
			text.WriteString(shellDialogText(child))
		}
		return text.String()
	case *container.Scroll:
		return shellDialogText(object.Content)
	case fyne.Widget:
		var text strings.Builder
		for _, child := range object.CreateRenderer().Objects() {
			text.WriteString(shellDialogText(child))
		}
		return text.String()
	}
	return ""
}
