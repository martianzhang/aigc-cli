//go:build darwin || linux

package audio

import "github.com/ebitengine/purego"

// openLibrary loads a shared library using purego.Dlopen (macOS/Linux).
func openLibrary(path string) (uintptr, error) {
	return purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_GLOBAL)
}
