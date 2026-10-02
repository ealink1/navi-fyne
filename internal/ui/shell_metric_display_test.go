package ui

import (
	"testing"

	transport "github.com/ealink1/navi-fyne/internal/infra/shell"
)

func TestShellMetricDisplayUsesValidatedSamples(t *testing.T) {
	metrics := transport.Metrics{System: "Linux", Load: "0.12 0.24 0.36 1/100 123", MemoryTotal: 2 << 30, MemoryUsed: 1 << 30, Uptime: "90000.00 80000.00", Disk: "Filesystem 1024-blocks Used Available Capacity Mounted on\n/dev/vda 41943040 10485760 31457280 25% /"}
	values, details, memory, disk := shellMetricDisplay(metrics)
	if values != [4]string{"0.12", "50%", "25%", "1d 1h"} || memory != 0.5 || disk != 0.25 || details[1] != "1.0 / 2.0 GiB" || details[2] != "10.0 / 40.0 GiB" {
		t.Fatalf("incorrect metric presentation: %v / %v / %v / %v", values, details, memory, disk)
	}
	for _, invalid := range []transport.Metrics{{}, {MemoryTotal: 1, MemoryUsed: 2, Uptime: "NaN", Disk: "x 1 1 0 NaN% /"}, {Uptime: "+Inf", Disk: "x 1 1 0 200% /"}} {
		values, _, memory, disk := shellMetricDisplay(invalid)
		if values != [4]string{"—", "—", "—", "—"} || memory != 0 || disk != 0 {
			t.Fatalf("invalid sample became a real value: %v", values)
		}
	}
}
