//go:build !darwin || !cgo

package ui

import "fyne.io/fyne/v2"

func installWorkspaceTitlebar(fyne.Window, func(int), func()) (func(int, bool), func(), bool) {
	return nil, nil, false
}
