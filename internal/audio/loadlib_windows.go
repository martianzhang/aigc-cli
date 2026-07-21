//go:build windows

package audio

import "golang.org/x/sys/windows"

// openLibrary loads a shared library using Windows LoadLibrary.
func openLibrary(path string) (uintptr, error) {
	h, err := windows.LoadLibrary(path)
	return uintptr(h), err
}
