package shell

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"time"
)

// Files reads SFTP v3 directory packets incrementally. Limits apply before
// allocating or accumulating server-controlled data, unlike ReadDir's full list.
func (r *Remote) Files(ctx context.Context, directory string) ([]File, error) {
	if len(directory) > 4096 || strings.ContainsRune(directory, 0) {
		return nil, errors.New("远程目录路径无效")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	session, closeSession, err := r.auxiliarySession(ctx)
	if err != nil {
		return nil, err
	}
	defer closeSession()
	input, err := session.StdinPipe()
	if err != nil {
		return nil, err
	}
	output, err := session.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := session.RequestSubsystem("sftp"); err != nil {
		return nil, err
	}
	wire := directoryWire{input: input, output: output}
	if err := wire.send(1, 0, nil); err != nil {
		return nil, err
	} // INIT carries protocol version instead of request ID.
	version, err := wire.read()
	if err != nil || len(version) < 5 || version[0] != 2 || binary.BigEndian.Uint32(version[1:5]) != 3 {
		return nil, errors.New("目录读取需要 SFTP v3")
	}
	response, err := wire.request(11, wireString(directory))
	if err != nil {
		return nil, err
	}
	if response[0] != 102 {
		return nil, directoryStatus(response)
	}
	handle, err := wireBytes(bytes.NewReader(response[5:]))
	if err != nil {
		return nil, err
	}
	defer wire.request(4, wireString(string(handle)))
	files := make([]File, 0, 64)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		response, err = wire.request(12, wireString(string(handle)))
		if err != nil {
			return nil, err
		}
		if response[0] == 101 {
			if len(response) >= 9 && binary.BigEndian.Uint32(response[5:9]) == 1 {
				return files, nil
			}
			return nil, directoryStatus(response)
		}
		if response[0] != 104 || len(response) < 9 {
			return nil, errors.New("SFTP 目录响应无效")
		}
		count := binary.BigEndian.Uint32(response[5:9])
		if count == 0 {
			return nil, errors.New("SFTP 返回了空目录批次")
		}
		if count > uint32(10000-len(files)) {
			return nil, errors.New("目录超过 10,000 项，请输入更具体的路径")
		}
		reader := bytes.NewReader(response[9:])
		for range count {
			entry, err := directoryEntry(reader)
			if err != nil {
				return nil, err
			}
			if entry.Name != "." && entry.Name != ".." {
				files = append(files, entry)
			}
		}
	}
}

type directoryWire struct {
	input  io.Writer
	output io.Reader
	id     uint32
}

func (w *directoryWire) send(kind byte, id uint32, payload []byte) error {
	if kind == 1 {
		id = 3
	}
	packet := make([]byte, 9+len(payload))
	binary.BigEndian.PutUint32(packet, uint32(5+len(payload)))
	packet[4] = kind
	binary.BigEndian.PutUint32(packet[5:], id)
	copy(packet[9:], payload)
	n, err := w.input.Write(packet)
	if err == nil && n != len(packet) {
		return io.ErrShortWrite
	}
	return err
}
func (w *directoryWire) read() ([]byte, error) {
	var header [4]byte
	if _, err := io.ReadFull(w.output, header[:]); err != nil {
		return nil, err
	}
	size := binary.BigEndian.Uint32(header[:])
	if size < 5 || size > 1<<20 {
		return nil, errors.New("SFTP 数据包长度无效或超过 1 MiB")
	}
	packet := make([]byte, int(size))
	_, err := io.ReadFull(w.output, packet)
	return packet, err
}
func (w *directoryWire) request(kind byte, payload []byte) ([]byte, error) {
	w.id++
	if err := w.send(kind, w.id, payload); err != nil {
		return nil, err
	}
	packet, err := w.read()
	if err != nil {
		return nil, err
	}
	if binary.BigEndian.Uint32(packet[1:5]) != w.id {
		return nil, errors.New("SFTP 请求编号不匹配")
	}
	return packet, nil
}
func wireString(value string) []byte {
	result := make([]byte, len(value)+4)
	binary.BigEndian.PutUint32(result, uint32(len(value)))
	copy(result[4:], value)
	return result
}
func wireBytes(reader *bytes.Reader) ([]byte, error) {
	var size uint32
	if err := binary.Read(reader, binary.BigEndian, &size); err != nil {
		return nil, err
	}
	if size > uint32(reader.Len()) {
		return nil, io.ErrUnexpectedEOF
	}
	raw := make([]byte, size)
	_, err := io.ReadFull(reader, raw)
	return raw, err
}
func directoryStatus(packet []byte) error {
	if len(packet) < 9 {
		return errors.New("SFTP 状态响应无效")
	}
	return fmt.Errorf("SFTP 操作失败，状态 %d", binary.BigEndian.Uint32(packet[5:9]))
}
func directoryEntry(reader *bytes.Reader) (File, error) {
	name, err := wireBytes(reader)
	if err != nil {
		return File{}, err
	}
	if len(name) > 4096 || strings.ContainsAny(string(name), "/\x00") {
		return File{}, errors.New("SFTP 文件名称无效")
	}
	if _, err := wireBytes(reader); err != nil {
		return File{}, err
	}
	var flags uint32
	if err := binary.Read(reader, binary.BigEndian, &flags); err != nil {
		return File{}, err
	}
	entry := File{Name: string(name)}
	if flags&1 != 0 {
		var size uint64
		if err := binary.Read(reader, binary.BigEndian, &size); err != nil {
			return entry, err
		}
		if size > 1<<63-1 {
			return entry, errors.New("SFTP 文件大小无效")
		}
		entry.Size = int64(size)
	}
	if flags&2 != 0 {
		var ids [2]uint32
		if err := binary.Read(reader, binary.BigEndian, &ids); err != nil {
			return entry, err
		}
	}
	if flags&4 != 0 {
		var mode uint32
		if err := binary.Read(reader, binary.BigEndian, &mode); err != nil {
			return entry, err
		}
		entry.Directory = mode&0170000 == 0040000
		m := fs.FileMode(mode & 0777)
		if entry.Directory {
			m |= fs.ModeDir
		}
		if mode&0170000 == 0120000 {
			m |= fs.ModeSymlink
		}
		entry.Mode = m.String()
	}
	if flags&8 != 0 {
		var times [2]uint32
		if err := binary.Read(reader, binary.BigEndian, &times); err != nil {
			return entry, err
		}
		entry.Modified = time.Unix(int64(times[1]), 0)
	}
	if flags&0x80000000 != 0 {
		var count uint32
		if err := binary.Read(reader, binary.BigEndian, &count); err != nil {
			return entry, err
		}
		if count > 1024 {
			return entry, errors.New("SFTP 扩展属性过多")
		}
		for range count {
			if _, err := wireBytes(reader); err != nil {
				return entry, err
			}
			if _, err := wireBytes(reader); err != nil {
				return entry, err
			}
		}
	}
	if flags & ^uint32(0x8000000f) != 0 {
		return entry, errors.New("SFTP 属性格式不支持")
	}
	return entry, nil
}
