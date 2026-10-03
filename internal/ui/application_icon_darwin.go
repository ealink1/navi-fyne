//go:build darwin && cgo

package ui

/*
#cgo LDFLAGS: -framework AppKit -framework Foundation
#include <stdlib.h>
void superlink_application_icon(const void *bytes, size_t length);
*/
import "C"

import "github.com/ealink1/super-link/internal/branding"

// GLFW does not set macOS Dock icons. Supply the same embedded artwork when
// running an unpackaged binary, where no bundle icon can be discovered.
func applyNativeApplicationIcon() {
	raw := branding.Icon().Content()
	buffer := C.CBytes(raw)
	defer C.free(buffer)
	C.superlink_application_icon(buffer, C.size_t(len(raw)))
}
