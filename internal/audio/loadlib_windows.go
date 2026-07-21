//go:build windows

package audio

import "golang.org/x/sys/windows"

// openLibrary loads a shared library using Windows LoadLibraryEx with
// LOAD_LIBRARY_SEARCH_DLL_LOAD_DIR, so the helper's own directory is searched
// for its transitive dependencies (sherpa-onnx-c-api.dll, onnxruntime.dll).
// This is scoped to this single call — no global state is modified.
func openLibrary(path string) (uintptr, error) {
	h, err := windows.LoadLibraryEx(path, 0,
		windows.LOAD_LIBRARY_SEARCH_DLL_LOAD_DIR|windows.LOAD_LIBRARY_SEARCH_DEFAULT_DIRS)
	return uintptr(h), err
}
