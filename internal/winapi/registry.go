package winapi

import (
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// RegString reads a string value (expanded if REG_EXPAND_SZ), or "".
func RegString(root registry.Key, path, name string) string {
	k, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	s, kind, err := k.GetStringValue(name)
	if err != nil {
		return ""
	}
	if kind == registry.EXPAND_SZ {
		if x, err := registry.ExpandString(s); err == nil {
			s = x
		}
	}
	return s
}

// RegDword reads a DWORD value.
func RegDword(root registry.Key, path, name string) (uint32, bool) {
	k, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
	if err != nil {
		return 0, false
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue(name)
	if err != nil {
		return 0, false
	}
	return uint32(v), true
}

// RegBinary reads a binary value.
func RegBinary(root registry.Key, path, name string) []byte {
	k, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
	if err != nil {
		return nil
	}
	defer k.Close()
	v, _, err := k.GetBinaryValue(name)
	if err != nil {
		return nil
	}
	return v
}

// ExpandEnv expands %VARIABLES% as Windows does.
func ExpandEnv(s string) string {
	if !strings.Contains(s, "%") {
		return s
	}
	if x, err := registry.ExpandString(s); err == nil {
		return x
	}
	return s
}

// UTF16 returns s as a NUL-terminated UTF-16 string, or nil if it holds a
// NUL.
func UTF16(s string) *uint16 {
	p, err := windows.UTF16PtrFromString(s)
	if err != nil {
		return nil
	}
	return p
}

// FileExists reports whether a file or folder is at path.
func FileExists(path string) bool {
	p := UTF16(path)
	if p == nil {
		return false
	}
	_, err := windows.GetFileAttributes(p)
	return err == nil
}
