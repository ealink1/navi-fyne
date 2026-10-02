package update

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const MaxExtractedBytes int64 = 2 << 30
const MaxFiles = 5000

// Extract rejects links, traversal, duplicate/case-colliding names, Windows
// device names and decompression bombs before committing a staged package.
func Extract(ctx context.Context, archive, destination string) error {
	if info, statErr := os.Lstat(destination); statErr == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("extraction destination must be a real empty directory")
		}
		entries, readErr := os.ReadDir(destination)
		if readErr != nil {
			return readErr
		}
		if len(entries) != 0 {
			return errors.New("extraction destination must be empty")
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return statErr
	}
	reader, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer reader.Close()
	if len(reader.File) == 0 || len(reader.File) > MaxFiles {
		return errors.New("invalid archive entry count")
	}
	seen := map[string]bool{}
	var declared uint64
	for _, file := range reader.File {
		name, err := safeName(file.Name)
		if err != nil {
			return err
		}
		key := strings.ToLower(name)
		if seen[key] {
			return errors.New("duplicate archive entry")
		}
		seen[key] = true
		if file.Mode()&os.ModeSymlink != 0 || (!file.Mode().IsRegular() && !file.FileInfo().IsDir()) {
			return errors.New("archive contains a link or special file")
		}
		if file.UncompressedSize64 > uint64(MaxExtractedBytes) {
			return errors.New("archive entry exceeds extraction budget")
		}
		declared += file.UncompressedSize64
		if declared > uint64(MaxExtractedBytes) {
			return errors.New("archive exceeds extraction budget")
		}
	}
	if err = os.MkdirAll(destination, 0700); err != nil {
		return err
	}
	complete := false
	defer func() {
		if !complete {
			_ = os.RemoveAll(destination)
		}
	}()
	var total int64
	for _, file := range reader.File {
		if err = ctx.Err(); err != nil {
			return err
		}
		name, _ := safeName(file.Name)
		target := filepath.Join(destination, filepath.FromSlash(name))
		if file.FileInfo().IsDir() {
			if err = os.MkdirAll(target, 0700); err != nil {
				return err
			}
			continue
		}
		if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return err
		}
		input, err := file.Open()
		if err != nil {
			return err
		}
		mode := os.FileMode(0600)
		if file.Mode()&0111 != 0 {
			mode = 0700
		}
		output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
		if err != nil {
			_ = input.Close()
			return err
		}
		copied, copyErr := io.Copy(output, io.LimitReader(input, MaxExtractedBytes-total+1))
		err = errors.Join(copyErr, input.Close(), output.Sync(), output.Close())
		if err != nil {
			return err
		}
		total += copied
		if total > MaxExtractedBytes {
			return errors.New("archive exceeds extraction budget")
		}
	}
	complete = true
	return nil
}
func safeName(name string) (string, error) {
	if name == "" || strings.ContainsAny(name, "\\:\x00") || strings.HasPrefix(name, "/") {
		return "", errors.New("unsafe archive path")
	}
	clean := path.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(name, "//") {
		return "", errors.New("unsafe archive path")
	}
	for _, part := range strings.Split(clean, "/") {
		if part == ".." || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") {
			return "", errors.New("unsafe archive path component")
		}
		stem := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
		if stem == "CON" || stem == "PRN" || stem == "AUX" || stem == "NUL" || len(stem) == 4 && (strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")) && stem[3] >= '1' && stem[3] <= '9' {
			return "", fmt.Errorf("reserved archive path %q", part)
		}
	}
	// Clean must not silently normalize embedded parent components.
	if strings.TrimSuffix(name, "/") != clean {
		return "", errors.New("non-canonical archive path")
	}
	return clean, nil
}
