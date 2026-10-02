package drivers

import (
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"errors"
	"os"
	"runtime"
)

// Check executable format and CPU before executing a downloaded driver.
func validateArchitecture(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("driver must be a regular executable file")
	}
	valid := false
	switch runtime.GOOS {
	case "darwin":
		file, err := macho.Open(path)
		if err != nil {
			return errors.New("driver is not a Mach-O executable")
		}
		defer file.Close()
		valid = file.Type == macho.TypeExec && (runtime.GOARCH == "arm64" && file.Cpu == macho.CpuArm64 || runtime.GOARCH == "amd64" && file.Cpu == macho.CpuAmd64)
	case "linux":
		file, err := elf.Open(path)
		if err != nil {
			return errors.New("driver is not an ELF executable")
		}
		defer file.Close()
		valid = (file.Type == elf.ET_EXEC || file.Type == elf.ET_DYN) && (runtime.GOARCH == "arm64" && file.Machine == elf.EM_AARCH64 || runtime.GOARCH == "amd64" && file.Machine == elf.EM_X86_64)
	case "windows":
		file, err := pe.Open(path)
		if err != nil {
			return errors.New("driver is not a PE executable")
		}
		defer file.Close()
		valid = runtime.GOARCH == "amd64" && file.Machine == pe.IMAGE_FILE_MACHINE_AMD64 || runtime.GOARCH == "arm64" && file.Machine == pe.IMAGE_FILE_MACHINE_ARM64
	}
	if !valid {
		return errors.New("driver CPU architecture does not match this application")
	}
	return nil
}
