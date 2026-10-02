package shell

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

const fixtureMetrics = "Linux\n__LOAD__\n0.1 0.2 0.3 1/10 42\n__MEM__\nMemTotal: 1000 kB\nMemAvailable: 400 kB\n__UPTIME__\n123.0 50.0\n__DISK__\nFilesystem Blocks Used Available Capacity Mounted\n/dev/root 100 20 80 20% /\n"

func TestMonitorParsingAndBound(t *testing.T) {
	m, err := parseMetrics(fixtureMetrics)
	if err != nil || m.MemoryTotal != 1024000 || m.MemoryUsed != 614400 {
		t.Fatal(m, err)
	}
	for _, raw := range []string{"bad", strings.Replace(fixtureMetrics, "Linux", "Darwin", 1), strings.Replace(fixtureMetrics, "400 kB", "2000 kB", 1)} {
		if _, err := parseMetrics(raw); err == nil {
			t.Fatal("invalid metrics accepted")
		}
	}
	b := &boundedOutput{limit: 3}
	if _, err := b.Write([]byte("1234")); err == nil || b.Len() != 0 {
		t.Fatal("output overflow")
	}
}

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) { return len(p) - 1, nil }
func TestStreamingCopyCancellationAndShortWrite(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := copyFile(ctx, io.Discard, strings.NewReader("data"), nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := copyFile(context.Background(), shortWriter{}, strings.NewReader("data"), nil); !errors.Is(err, io.ErrShortWrite) {
		t.Fatal(err)
	}
}
