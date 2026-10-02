package shell

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/ealink1/navi-fyne/internal/domain"
	"golang.org/x/crypto/ssh"
)

// HostKeyError prevents silent trust and distinguishes changed server identities.
type HostKeyError struct {
	Fingerprint string
	Changed     bool
}

func (e *HostKeyError) Error() string {
	if e.Changed {
		return "SSH 主机指纹发生变化，连接已阻止，请核实服务器身份"
	}
	return "首次连接需要核实 SSH 主机指纹：" + e.Fingerprint
}

// Remote is an SSH PTY, with auxiliary channels sharing its verified client.
type Remote struct {
	client  *ssh.Client
	session *ssh.Session
	input   io.WriteCloser
	output  *io.PipeReader
	writer  *io.PipeWriter
	done    chan struct{}
	once    sync.Once
}

// OpenSSH authenticates with a pinned host key and opens an interactive PTY.
func OpenSSH(ctx context.Context, h domain.ShellHost) (*Remote, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if err := h.Validate(); err != nil {
		return nil, err
	}
	auth, err := sshAuth(h)
	if err != nil {
		return nil, err
	}
	address := net.JoinHostPort(h.Host, strconv.Itoa(h.Port))
	connection, err := (&net.Dialer{Timeout: 15 * time.Second}).DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, fmt.Errorf("连接 SSH：%w", err)
	}
	stop := context.AfterFunc(ctx, func() { connection.Close() })
	defer stop()
	if err := connection.SetDeadline(time.Now().Add(15 * time.Second)); err != nil {
		connection.Close()
		return nil, err
	}
	config := &ssh.ClientConfig{User: h.User, Auth: auth, HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
		fingerprint := ssh.FingerprintSHA256(key)
		if subtle.ConstantTimeCompare([]byte(h.Fingerprint), []byte(fingerprint)) != 1 {
			return &HostKeyError{Fingerprint: fingerprint, Changed: h.Fingerprint != ""}
		}
		return nil
	}}
	clientConn, channels, requests, err := ssh.NewClientConn(connection, address, config)
	if err != nil {
		connection.Close()
		return nil, fmt.Errorf("SSH 握手：%w", err)
	}
	client := ssh.NewClient(clientConn, channels, requests)
	if err := connection.SetDeadline(time.Time{}); err != nil {
		client.Close()
		return nil, err
	}
	remote, err := openPTY(client)
	if err != nil {
		client.Close()
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, errors.Join(err, remote.Close())
	}
	return remote, nil
}

func sshAuth(h domain.ShellHost) ([]ssh.AuthMethod, error) {
	if h.KeyPath == "" {
		return []ssh.AuthMethod{ssh.Password(h.Password)}, nil
	}
	file, err := os.Open(h.KeyPath)
	if err != nil {
		return nil, fmt.Errorf("读取私钥：%w", err)
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, (2<<20)+1))
	defer clear(raw)
	if err != nil || len(raw) > 2<<20 {
		return nil, errors.New("私钥无法读取或超过 2 MiB")
	}
	var signer ssh.Signer
	if h.Passphrase != "" {
		signer, err = ssh.ParsePrivateKeyWithPassphrase(raw, []byte(h.Passphrase))
	} else {
		signer, err = ssh.ParsePrivateKey(raw)
	}
	if err != nil {
		return nil, errors.New("无法解析私钥，请检查格式和私钥口令")
	}
	return []ssh.AuthMethod{ssh.PublicKeys(signer)}, nil
}

func openPTY(client *ssh.Client) (*Remote, error) {
	session, err := client.NewSession()
	if err != nil {
		return nil, err
	}
	input, err := session.StdinPipe()
	if err != nil {
		session.Close()
		return nil, err
	}
	reader, writer := io.Pipe()
	session.Stdout, session.Stderr = writer, writer
	if err := session.RequestPty("xterm-256color", 24, 80, ssh.TerminalModes{ssh.ECHO: 1}); err != nil {
		session.Close()
		reader.Close()
		writer.Close()
		return nil, err
	}
	if err := session.Shell(); err != nil {
		session.Close()
		reader.Close()
		writer.Close()
		return nil, err
	}
	r := &Remote{client: client, session: session, input: input, output: reader, writer: writer, done: make(chan struct{})}
	go func() {
		defer close(r.done)
		err := session.Wait()
		writer.CloseWithError(err)
	}()
	return r, nil
}

// Read receives terminal output from the SSH PTY.
func (r *Remote) Read(p []byte) (int, error) { return r.output.Read(p) }

// Write sends encoded terminal input to the SSH PTY.
func (r *Remote) Write(p []byte) (int, error) { return r.input.Write(p) }

// Resize synchronizes the remote PTY geometry.
func (r *Remote) Resize(cols, rows int) error { return r.session.WindowChange(rows, cols) }

// Close interrupts transport I/O and joins the PTY's waiter.
func (r *Remote) Close() error {
	var err error
	r.once.Do(func() {
		// Close the transport first so stalled channels and pipe copies unblock.
		err = r.client.Close()
		err = errors.Join(err, r.output.Close(), r.writer.Close())
		<-r.done
	})
	return err
}
