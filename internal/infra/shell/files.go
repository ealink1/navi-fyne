package shell

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/sftp"
)

// File describes a remote directory entry without reading file contents.
type File struct {
	Name, Mode string
	Size       int64
	Modified   time.Time
	Directory  bool
}

func (r *Remote) fileClient(ctx context.Context) (*sftp.Client, func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	session, closeSession, err := r.auxiliarySession(ctx)
	if err != nil {
		return nil, nil, err
	}
	input, err := session.StdinPipe()
	if err != nil {
		closeSession()
		return nil, nil, err
	}
	output, err := session.StdoutPipe()
	if err != nil {
		closeSession()
		return nil, nil, err
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		closeSession()
		return nil, nil, err
	}
	if err := session.RequestSubsystem("sftp"); err != nil {
		closeSession()
		return nil, nil, err
	}
	done := make(chan struct{})
	go func() { defer close(done); io.Copy(io.Discard, stderr) }()
	client, err := sftp.NewClientPipe(output, auxiliaryWriter{Writer: input, close: session.Close})
	if err != nil {
		closeSession()
		<-done
		return nil, nil, err
	}
	return client, func() { closeSession(); client.Close(); <-done }, nil
}

type auxiliaryWriter struct {
	io.Writer
	close func() error
}

func (w auxiliaryWriter) Close() error { return w.close() }

// Download streams a file into a new destination and removes incomplete output.
func (r *Remote) Download(ctx context.Context, source, destination string, progress func(int64)) error {
	client, closeClient, err := r.fileClient(ctx)
	if err != nil {
		return err
	}
	defer closeClient()
	remote, err := client.Open(source)
	if err != nil {
		return err
	}
	defer remote.Close()
	local, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	err = copyFile(ctx, local, remote, progress)
	err = errors.Join(err, local.Sync(), local.Close())
	if err != nil {
		return errors.Join(err, os.Remove(destination))
	}
	return nil
}

// Upload streams into a new remote name, without silently overwriting files.
func (r *Remote) Upload(ctx context.Context, source, destination string, progress func(int64)) error {
	client, closeClient, err := r.fileClient(ctx)
	if err != nil {
		return err
	}
	defer closeClient()
	local, err := os.Open(source)
	if err != nil {
		return err
	}
	defer local.Close()
	info, err := local.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("只支持上传普通文件")
	}
	if path.Base(destination) == "." || path.Base(destination) == "/" {
		return errors.New("请输入远程文件名称")
	}
	if _, err := client.Stat(destination); err == nil {
		return errors.New("远程文件已存在，请使用不同名称")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	staging := path.Join(path.Dir(destination), ".navi-upload-"+uuid.NewString())
	remote, err := client.OpenFile(staging, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
	if err != nil {
		return err
	}
	err = copyFile(ctx, remote, local, progress)
	err = errors.Join(err, remote.Close())
	if err == nil {
		err = client.Rename(staging, destination)
	} // Standard SFTP v3 rename refuses an existing destination.
	if err != nil {
		cleanup := r.removeIncompleteUpload(staging)
		return errors.Join(err, cleanup)
	}
	return nil
}

func (r *Remote) removeIncompleteUpload(filename string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	client, closeClient, err := r.fileClient(ctx)
	if err != nil {
		return err
	}
	defer closeClient()
	return client.Remove(filename)
}

func copyFile(ctx context.Context, destination io.Writer, source io.Reader, progress func(int64)) error {
	buffer := make([]byte, 64<<10)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, readErr := source.Read(buffer)
		if n > 0 {
			written, err := destination.Write(buffer[:n])
			if err != nil {
				return err
			}
			if written != n {
				return io.ErrShortWrite
			}
			total += int64(n)
			if progress != nil {
				progress(total)
			}
		}
		if errors.Is(readErr, io.EOF) {
			return nil
		}
		if readErr != nil {
			return fmt.Errorf("传输文件：%w", readErr)
		}
	}
}
