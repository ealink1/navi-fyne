// Package branding provides SuperLink's shared application artwork.
package branding

import (
	_ "embed"

	"fyne.io/fyne/v2"
)

//go:embed assets/superlink.png
var iconPNG []byte

// Icon returns the user-supplied SL and gear mark for application windows.
func Icon() fyne.Resource {
	return fyne.NewStaticResource("superlink.png", iconPNG)
}
