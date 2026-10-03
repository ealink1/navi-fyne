// Command app-icon converts the supplied artwork into runtime and macOS icons.
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"

	"golang.org/x/image/draw"
)

func main() {
	if err := generate("internal/branding/assets"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func generate(directory string) error {
	file, err := os.Open(filepath.Join(directory, "superlink-rounded.png"))
	if err != nil {
		return err
	}
	source, decodeErr := png.Decode(file)
	closeErr := file.Close()
	if decodeErr != nil {
		return decodeErr
	}
	if closeErr != nil {
		return closeErr
	}
	var chunks bytes.Buffer
	for _, size := range []struct {
		edge int
		kind string
	}{{16, "icp4"}, {32, "icp5"}, {64, "icp6"}, {128, "ic07"}, {256, "ic08"}, {512, "ic09"}, {1024, "ic10"}} {
		pixels := image.NewNRGBA(image.Rect(0, 0, size.edge, size.edge))
		draw.CatmullRom.Scale(pixels, pixels.Bounds(), source, source.Bounds(), draw.Src, nil)
		var encoded bytes.Buffer
		if err := png.Encode(&encoded, pixels); err != nil {
			return err
		}
		if size.edge == 256 {
			if err := os.WriteFile(filepath.Join(directory, "superlink.png"), encoded.Bytes(), 0644); err != nil {
				return err
			}
		}
		chunks.WriteString(size.kind)
		if err := binary.Write(&chunks, binary.BigEndian, uint32(encoded.Len()+8)); err != nil {
			return err
		}
		chunks.Write(encoded.Bytes())
	}
	var icon bytes.Buffer
	icon.WriteString("icns")
	if err := binary.Write(&icon, binary.BigEndian, uint32(chunks.Len()+8)); err != nil {
		return err
	}
	icon.Write(chunks.Bytes())
	return os.WriteFile(filepath.Join(directory, "superlink.icns"), icon.Bytes(), 0644)
}
