//go:build windows

package detector

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

var (
	modVersion                  = syscall.NewLazyDLL("version.dll")
	procGetFileVersionInfoSizeW = modVersion.NewProc("GetFileVersionInfoSizeW")
	procGetFileVersionInfoW     = modVersion.NewProc("GetFileVersionInfoW")
	procVerQueryValueW          = modVersion.NewProc("VerQueryValueW")
)

type vsFixedFileInfo struct {
	Signature        uint32
	StrucVersion     uint32
	FileVersionMS    uint32
	FileVersionLS    uint32
	ProductVersionMS uint32
	ProductVersionLS uint32
	FileFlagsMask    uint32
	FileFlags        uint32
	FileOS           uint32
	FileType         uint32
	FileSubtype      uint32
	FileDateMS       uint32
	FileDateLS       uint32
}

func detectPE(path string) string {
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return ""
	}

	size, _, _ := procGetFileVersionInfoSizeW.Call(uintptr(unsafe.Pointer(pathPtr)), 0)
	if size == 0 {
		return ""
	}

	buf := make([]byte, size)
	ret, _, _ := procGetFileVersionInfoW.Call(uintptr(unsafe.Pointer(pathPtr)), 0, size, uintptr(unsafe.Pointer(&buf[0])))
	if ret == 0 {
		return ""
	}

	root, _ := syscall.UTF16PtrFromString(`\`)
	var fixedInfo *vsFixedFileInfo
	var fixedLen uint32
	ret, _, _ = procVerQueryValueW.Call(
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(root)),
		uintptr(unsafe.Pointer(&fixedInfo)),
		uintptr(unsafe.Pointer(&fixedLen)),
	)
	if ret == 0 || fixedInfo == nil {
		return ""
	}

	major := fixedInfo.FileVersionMS >> 16
	minor := fixedInfo.FileVersionMS & 0xFFFF
	patch := fixedInfo.FileVersionLS >> 16
	build := fixedInfo.FileVersionLS & 0xFFFF

	if major == 0 && minor == 0 && patch == 0 {
		return ""
	}
	if major == 1 && minor == 0 && patch == 0 && build == 0 {
		return ""
	}

	if build == 0 {
		return fmt.Sprintf("%d.%d.%d", major, minor, patch)
	}
	return fmt.Sprintf("%d.%d.%d.%d", major, minor, patch, build)
}

func resolveChocoShimPlatform(path string) string {
	lower := strings.ToLower(filepath.Clean(path))
	chocoSuffix := string(filepath.Separator) + "bin" + string(filepath.Separator)
	idx := strings.LastIndex(lower, "chocolatey"+chocoSuffix)
	if idx < 0 {
		return ""
	}

	chocoRoot := filepath.Clean(path[:idx+len("chocolatey")])
	libDir := filepath.Join(chocoRoot, "lib")
	binName := filepath.Base(path)

	var found string
	_ = filepath.WalkDir(libDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Base(p), binName) {
			if !strings.EqualFold(filepath.Dir(p), filepath.Join(chocoRoot, "bin")) {
				if info, err := os.Stat(p); err == nil && info.Mode().IsRegular() {
					found = p
					return filepath.SkipAll
				}
			}
		}
		return nil
	})
	return found
}
