//go:build windows

package detector

import (
	"fmt"
	"syscall"
	"unsafe"
)

var (
	modVersion                      = syscall.NewLazyDLL("version.dll")
	procGetFileVersionInfoSizeW     = modVersion.NewProc("GetFileVersionInfoSizeW")
	procGetFileVersionInfoW         = modVersion.NewProc("GetFileVersionInfoW")
	procVerQueryValueW              = modVersion.NewProc("VerQueryValueW")
)

// vsFixedFileInfo mirrors the Win32 VS_FIXEDFILEINFO structure.
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

// detectPE reads the VS_FIXEDFILEINFO version resource from a Windows PE
// binary using version.dll syscalls. Returns a version string like "2.53.0"
// or "" if no version resource exists or the version is all zeros.
//
// This never executes the target binary.
func detectPE(path string) string {
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return ""
	}

	// Get size of version info block.
	size, _, _ := procGetFileVersionInfoSizeW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		0,
	)
	if size == 0 {
		return ""
	}

	// Read version info into buffer.
	buf := make([]byte, size)
	ret, _, _ := procGetFileVersionInfoW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		0,
		size,
		uintptr(unsafe.Pointer(&buf[0])),
	)
	if ret == 0 {
		return ""
	}

	// Query the root (\) to get VS_FIXEDFILEINFO.
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

	// Skip bogus versions (all zeros, or generic 1.0.0.0 / 0.0.0.0).
	if major == 0 && minor == 0 && patch == 0 {
		return ""
	}

	// Omit trailing .0 build number for cleaner display.
	if build == 0 {
		return fmt.Sprintf("%d.%d.%d", major, minor, patch)
	}
	return fmt.Sprintf("%d.%d.%d.%d", major, minor, patch, build)
}
