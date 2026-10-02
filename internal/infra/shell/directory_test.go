package shell

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"
)

func TestDirectoryWireRejectsUnboundedAndTruncatedPackets(t *testing.T) {
	for _, size := range []uint32{0, 4, (1 << 20) + 1} {
		var header [4]byte
		binary.BigEndian.PutUint32(header[:], size)
		wire := directoryWire{output: bytes.NewReader(header[:])}
		if _, err := wire.read(); err == nil {
			t.Fatal("oversized packet accepted", size)
		}
	}
	reader := bytes.NewReader([]byte{0, 0, 0, 100, 1})
	if _, err := wireBytes(reader); err == nil {
		t.Fatal("truncated string accepted")
	}
	var out bytes.Buffer
	wire := directoryWire{input: &out, output: bytes.NewReader([]byte{0, 0, 0, 5, 101, 0, 0, 0, 2})}
	if _, err := wire.request(11, nil); err == nil {
		t.Fatal("mismatched response accepted")
	}
}
func TestDirectoryEntryParsingAndTraversalRejection(t *testing.T) {
	packet := append(wireString("文件"), wireString("long name")...)
	packet = binary.BigEndian.AppendUint32(packet, 13)
	packet = binary.BigEndian.AppendUint64(packet, 42)
	packet = binary.BigEndian.AppendUint32(packet, 0100640)
	packet = binary.BigEndian.AppendUint32(packet, 10)
	packet = binary.BigEndian.AppendUint32(packet, 20)
	entry, err := directoryEntry(bytes.NewReader(packet))
	if err != nil || entry.Name != "文件" || entry.Size != 42 || entry.Mode != "-rw-r-----" || entry.Modified.Unix() != 20 {
		t.Fatal(entry, err)
	}
	for _, name := range []string{"../outside", "a\x00b"} {
		if _, err := directoryEntry(bytes.NewReader(wireString(name))); err == nil {
			t.Fatal("unsafe name accepted")
		}
	}
	if _, err := directoryEntry(bytes.NewReader(packet[:len(packet)-1])); err != io.ErrUnexpectedEOF {
		t.Fatal("truncated attributes accepted", err)
	}
}
