package onnxrt

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLibPath_notFound(t *testing.T) {
	dir := t.TempDir()
	_, err := LibPath(dir)
	if err == nil {
		t.Fatal("expected error for empty directory")
	}
}

func platformLibName() string {
	switch runtime.GOOS {
	case "darwin":
		return "libonnxruntime.dylib"
	case "linux":
		return "libonnxruntime.so"
	default:
		return "onnxruntime.dll"
	}
}

func TestLibPath_found(t *testing.T) {
	dir := t.TempDir()
	libName := platformLibName()
	fakeLib := filepath.Join(dir, libName)
	if err := os.WriteFile(fakeLib, []byte("fake"), 0644); err != nil {
		t.Fatal(err)
	}
	path, err := LibPath(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != fakeLib {
		t.Fatalf("got %q, want %q", path, fakeLib)
	}
}

func TestLibPath_prefersCPUOnly(t *testing.T) {
	dir := t.TempDir()
	// LibPath only looks for the single main library per platform.
	// GPU providers are loaded dynamically alongside it.
	libName := platformLibName()
	lib := filepath.Join(dir, libName)
	os.WriteFile(lib, []byte("data"), 0644)

	path, err := LibPath(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != lib {
		t.Fatalf("got %q, want %q", path, lib)
	}
}

func TestVersion_const(t *testing.T) {
	if Version == "" {
		t.Fatal("Version must not be empty")
	}
}

func expectedORTArch() string {
	switch runtime.GOOS {
	case "windows":
		if runtime.GOARCH == "arm64" {
			return "arm64"
		}
		return "x64"
	case "darwin":
		if runtime.GOARCH == "amd64" {
			return "x64"
		}
		return "arm64"
	default: // linux
		if runtime.GOARCH == "arm64" {
			return "aarch64"
		}
		return "x64"
	}
}

func TestGetORTDownloadInfo(t *testing.T) {
	info := getORTDownloadInfo()
	arch := expectedORTArch()

	var wantURL, wantArchive, wantLib, wantInternal string
	switch runtime.GOOS {
	case "windows":
		wantURL = fmt.Sprintf("%s/onnxruntime-win-%s-%s.zip", modelsBaseURL, arch, Version)
		wantArchive = fmt.Sprintf("onnxruntime-%s.zip", Version)
		wantLib = "onnxruntime.dll"
		wantInternal = fmt.Sprintf("onnxruntime-win-%s-%s/lib/onnxruntime.dll", arch, Version)
	case "darwin":
		wantURL = fmt.Sprintf("%s/onnxruntime-osx-%s-%s.tgz", modelsBaseURL, arch, Version)
		wantArchive = fmt.Sprintf("onnxruntime-%s.tgz", Version)
		wantLib = "libonnxruntime.dylib"
		wantInternal = fmt.Sprintf("onnxruntime-osx-%s-%s/lib/libonnxruntime.dylib", arch, Version)
	default: // linux
		wantURL = fmt.Sprintf("%s/onnxruntime-linux-%s-%s.tgz", modelsBaseURL, arch, Version)
		wantArchive = fmt.Sprintf("onnxruntime-%s.tgz", Version)
		wantLib = "libonnxruntime.so"
		wantInternal = fmt.Sprintf("onnxruntime-linux-%s-%s/lib/libonnxruntime.so", arch, Version)
	}

	if info.url != wantURL {
		t.Errorf("url = %q, want %q", info.url, wantURL)
	}
	if !strings.HasPrefix(info.url, modelsBaseURL) {
		t.Errorf("url = %q, want prefix %q", info.url, modelsBaseURL)
	}
	if info.archiveName != wantArchive {
		t.Errorf("archiveName = %q, want %q", info.archiveName, wantArchive)
	}
	if info.libName != wantLib {
		t.Errorf("libName = %q, want %q", info.libName, wantLib)
	}
	if info.internalPath != wantInternal {
		t.Errorf("internalPath = %q, want %q", info.internalPath, wantInternal)
	}
	if !strings.Contains(info.internalPath, arch) || !strings.Contains(info.internalPath, Version) {
		t.Errorf("internalPath = %q, want it to contain arch %q and version %q", info.internalPath, arch, Version)
	}
}

func TestGPUORTDownloadInfo(t *testing.T) {
	info := getGPUORTDownloadInfo()

	switch runtime.GOOS {
	case "darwin":
		if info != nil {
			t.Fatalf("getGPUORTDownloadInfo() = %+v, want nil on darwin", info)
		}
	case "linux":
		if runtime.GOARCH != "amd64" {
			if info != nil {
				t.Fatalf("getGPUORTDownloadInfo() = %+v, want nil on linux/%s", info, runtime.GOARCH)
			}
			return
		}
		if info == nil {
			t.Fatal("getGPUORTDownloadInfo() = nil, want non-nil on linux/amd64")
		}
		if info.libName != "libonnxruntime_providers_cuda.so" {
			t.Errorf("libName = %q, want %q", info.libName, "libonnxruntime_providers_cuda.so")
		}
		if !strings.HasPrefix(info.url, modelsBaseURL) {
			t.Errorf("url = %q, want prefix %q", info.url, modelsBaseURL)
		}
		if !strings.HasSuffix(info.url, ".tgz") {
			t.Errorf("url = %q, want .tgz archive", info.url)
		}
		if !strings.HasSuffix(info.archiveName, ".tgz") {
			t.Errorf("archiveName = %q, want .tgz archive", info.archiveName)
		}
	case "windows":
		if info == nil {
			t.Fatal("getGPUORTDownloadInfo() = nil, want non-nil on windows")
		}
		if info.libName != "onnxruntime_providers_cuda.dll" {
			t.Errorf("libName = %q, want %q", info.libName, "onnxruntime_providers_cuda.dll")
		}
		if !strings.HasPrefix(info.url, modelsBaseURL) {
			t.Errorf("url = %q, want prefix %q", info.url, modelsBaseURL)
		}
		if !strings.HasSuffix(info.url, ".zip") {
			t.Errorf("url = %q, want .zip archive", info.url)
		}
		if !strings.HasSuffix(info.archiveName, ".zip") {
			t.Errorf("archiveName = %q, want .zip archive", info.archiveName)
		}
	default:
		if info != nil {
			t.Fatalf("getGPUORTDownloadInfo() = %+v, want nil on %s", info, runtime.GOOS)
		}
	}
}

func TestGpuLibName(t *testing.T) {
	var want string
	switch runtime.GOOS {
	case "linux":
		want = "libonnxruntime_providers_cuda.so"
	case "windows":
		want = "onnxruntime_providers_cuda.dll"
	default: // darwin and any other platform
		want = ""
	}
	if got := gpuLibName(); got != want {
		t.Fatalf("gpuLibName() = %q, want %q (GOOS=%s)", got, want, runtime.GOOS)
	}
}

func makeZip(t *testing.T, path string, entries map[string]string) {
	t.Helper()

	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	zw := zip.NewWriter(f)
	for name, content := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip create %q: %v", name, err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("zip write %q: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close zip file: %v", err)
	}
}

type tgzEntry struct {
	name     string
	content  string
	typeflag byte
	linkname string
}

func makeTGZ(t *testing.T, path string, entries []tgzEntry) {
	t.Helper()

	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create tgz: %v", err)
	}
	gzw := gzip.NewWriter(f)
	tw := tar.NewWriter(gzw)

	for _, e := range entries {
		typeflag := e.typeflag
		if typeflag == 0 {
			typeflag = tar.TypeReg
		}
		hdr := &tar.Header{
			Name:     e.name,
			Mode:     0644,
			Typeflag: typeflag,
			Linkname: e.linkname,
		}
		if typeflag == tar.TypeReg {
			hdr.Size = int64(len(e.content))
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("tar header %q: %v", e.name, err)
		}
		if typeflag == tar.TypeReg {
			if _, err := tw.Write([]byte(e.content)); err != nil {
				t.Fatalf("tar write %q: %v", e.name, err)
			}
		}
	}

	if err := tw.Close(); err != nil {
		t.Fatalf("tar close: %v", err)
	}
	if err := gzw.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close tgz file: %v", err)
	}
}

func countDirEntries(t *testing.T, dir string) int {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir %q: %v", dir, err)
	}
	return len(entries)
}

const testLibContent = "fake-onnx-runtime-binary"

func TestExtractZip(t *testing.T) {
	dir := t.TempDir()
	modelsDir := t.TempDir()
	internalPath := "onnxruntime-test/lib/libtest.so"
	archive := filepath.Join(dir, "runtime.zip")
	makeZip(t, archive, map[string]string{
		"onnxruntime-test/README.md": "docs",
		internalPath:                 testLibContent,
	})

	if err := extractZip(archive, modelsDir, internalPath, "libtest.so"); err != nil {
		t.Fatalf("extractZip: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(modelsDir, "libtest.so"))
	if err != nil {
		t.Fatalf("extracted library missing: %v", err)
	}
	if string(got) != testLibContent {
		t.Fatalf("extracted content = %q, want %q", got, testLibContent)
	}
}

func TestExtractTGZ(t *testing.T) {
	dir := t.TempDir()
	modelsDir := t.TempDir()
	internalPath := "onnxruntime-test/lib/libtest.so"
	archive := filepath.Join(dir, "runtime.tgz")
	// Entries in real tarballs are often "./"-prefixed; extractTGZ must strip it.
	makeTGZ(t, archive, []tgzEntry{
		{name: "./onnxruntime-test/README.md", content: "docs"},
		{name: "./" + internalPath, content: testLibContent},
	})

	if err := extractTGZ(archive, modelsDir, internalPath, "libtest.so"); err != nil {
		t.Fatalf("extractTGZ: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(modelsDir, "libtest.so"))
	if err != nil {
		t.Fatalf("extracted library missing: %v", err)
	}
	if string(got) != testLibContent {
		t.Fatalf("extracted content = %q, want %q", got, testLibContent)
	}
}

func TestExtractZip_notFound(t *testing.T) {
	dir := t.TempDir()
	modelsDir := t.TempDir()
	archive := filepath.Join(dir, "runtime.zip")
	makeZip(t, archive, map[string]string{
		"onnxruntime-test/lib/libtest.so": testLibContent,
	})

	err := extractZip(archive, modelsDir, "onnxruntime-test/lib/missing.so", "missing.so")
	if err == nil {
		t.Fatal("expected error when internalPath is absent from zip")
	}
}

func TestExtractTGZ_notFound(t *testing.T) {
	dir := t.TempDir()
	modelsDir := t.TempDir()
	archive := filepath.Join(dir, "runtime.tgz")
	makeTGZ(t, archive, []tgzEntry{
		{name: "onnxruntime-test/lib/libtest.so", content: testLibContent},
	})

	err := extractTGZ(archive, modelsDir, "onnxruntime-test/lib/missing.so", "missing.so")
	if err == nil {
		t.Fatal("expected error when internalPath is absent from tgz")
	}
}

func TestExtractAllTGZ(t *testing.T) {
	dir := t.TempDir()
	modelsDir := t.TempDir()
	archive := filepath.Join(dir, "runtime-gpu.tgz")
	makeTGZ(t, archive, []tgzEntry{
		{name: "pkg/lib/libonnxruntime.so", content: "runtime"},
		{name: "pkg/lib/libonnxruntime_providers_cuda.so", content: "cuda"},
		{name: "pkg/lib/libonnxruntime_providers_shared.so", content: "shared"},
		{name: "pkg/lib/readme.txt", content: "not a shared library"},
		{name: "pkg/lib/libtool.so", typeflag: tar.TypeSymlink, linkname: "libonnxruntime.so"},
		{name: "pkg/bin/liboutside.so", content: "outside lib/"},
	})

	if err := extractAllFromTGZ(archive, modelsDir); err != nil {
		t.Fatalf("extractAllFromTGZ: %v", err)
	}

	if got := countDirEntries(t, modelsDir); got != 3 {
		t.Fatalf("extracted %d files, want 3", got)
	}
	for _, name := range []string{
		"libonnxruntime.so",
		"libonnxruntime_providers_cuda.so",
		"libonnxruntime_providers_shared.so",
	} {
		if _, err := os.Stat(filepath.Join(modelsDir, name)); err != nil {
			t.Errorf("expected %s to be extracted: %v", name, err)
		}
	}
	for _, name := range []string{"readme.txt", "liboutside.so", "libtool.so"} {
		if _, err := os.Stat(filepath.Join(modelsDir, name)); err == nil {
			t.Errorf("%s should not have been extracted", name)
		}
	}

	got, err := os.ReadFile(filepath.Join(modelsDir, "libonnxruntime_providers_shared.so"))
	if err != nil {
		t.Fatalf("read extracted provider lib: %v", err)
	}
	if string(got) != "shared" {
		t.Fatalf("provider lib content = %q, want %q", got, "shared")
	}
}

func TestExtractAllZip(t *testing.T) {
	dir := t.TempDir()
	modelsDir := t.TempDir()
	archive := filepath.Join(dir, "runtime-gpu.zip")
	makeZip(t, archive, map[string]string{
		"pkg/lib/onnxruntime.dll":                  "runtime",
		"pkg/lib/onnxruntime_providers_cuda.dll":   "cuda",
		"pkg/lib/onnxruntime_providers_shared.dll": "shared",
		"pkg/lib/readme.txt":                       "not a shared library",
		"pkg/lib/libonnxruntime.so":                "not a dll",
	})

	if err := extractAllFromZip(archive, modelsDir); err != nil {
		t.Fatalf("extractAllFromZip: %v", err)
	}

	if got := countDirEntries(t, modelsDir); got != 3 {
		t.Fatalf("extracted %d files, want 3", got)
	}
	for _, name := range []string{
		"onnxruntime.dll",
		"onnxruntime_providers_cuda.dll",
		"onnxruntime_providers_shared.dll",
	} {
		if _, err := os.Stat(filepath.Join(modelsDir, name)); err != nil {
			t.Errorf("expected %s to be extracted: %v", name, err)
		}
	}
	for _, name := range []string{"readme.txt", "libonnxruntime.so"} {
		if _, err := os.Stat(filepath.Join(modelsDir, name)); err == nil {
			t.Errorf("%s should not have been extracted", name)
		}
	}

	got, err := os.ReadFile(filepath.Join(modelsDir, "onnxruntime_providers_cuda.dll"))
	if err != nil {
		t.Fatalf("read extracted provider dll: %v", err)
	}
	if string(got) != "cuda" {
		t.Fatalf("provider dll content = %q, want %q", got, "cuda")
	}
}

// --- pinned digest verification ---

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func TestPinnedArchiveDigests(t *testing.T) {
	if len(ortAssetSHA256) == 0 {
		t.Fatal("ortAssetSHA256 is empty: no runtime archive could be installed")
	}
	for name, sum := range ortAssetSHA256 {
		if len(sum) != 64 {
			t.Errorf("%s: digest %q is not 64 hex chars", name, sum)
		}
		if _, err := hex.DecodeString(sum); err != nil {
			t.Errorf("%s: digest %q is not hex: %v", name, sum, err)
		}
		if sum != strings.ToLower(sum) {
			t.Errorf("%s: digest %q must be lowercase", name, sum)
		}
		if !strings.Contains(name, Version) {
			t.Errorf("%s: archive name must carry the pinned version %s", name, Version)
		}
		if !strings.HasSuffix(name, ".tgz") && !strings.HasSuffix(name, ".zip") {
			t.Errorf("%s: unexpected archive extension", name)
		}
	}
}

func TestGetORTDownloadInfo_usesPinnedDigest(t *testing.T) {
	info := getORTDownloadInfo()
	published := map[string]bool{
		"darwin/arm64":  true,
		"linux/amd64":   true,
		"windows/amd64": true,
	}
	key := runtime.GOOS + "/" + runtime.GOARCH

	if published[key] {
		if info.sha256 == "" {
			t.Fatalf("%s is published but getORTDownloadInfo() has no pinned digest", key)
		}
		if got := ortAssetSHA256[path.Base(info.url)]; got != info.sha256 {
			t.Errorf("sha256 = %q, want table entry %q for %s", info.sha256, got, path.Base(info.url))
		}
		return
	}
	if info.sha256 != "" {
		t.Errorf("%s has no published archive, but digest %q was pinned", key, info.sha256)
	}
}

func TestInstallArchive_rejectsUnpinnedArchive(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_, _ = w.Write([]byte("unverified"))
	}))
	defer srv.Close()

	info := ortDownloadInfo{
		url:         srv.URL + "/onnxruntime-test.tgz",
		archiveName: "runtime.tgz",
		libName:     "libtest.so",
	}
	err := installArchive(t.TempDir(), info, false, extractCPUArchive)
	if err == nil {
		t.Fatal("expected error for an archive without a pinned digest")
	}
	if !strings.Contains(err.Error(), "pinned SHA-256") {
		t.Fatalf("error = %q, want it to mention the missing pinned SHA-256", err)
	}
	if hits != 0 {
		t.Fatalf("server received %d requests, want 0: unverified archives must not be downloaded", hits)
	}
}

func TestInstallArchive_rejectsTamperedArchive(t *testing.T) {
	dir := t.TempDir()
	modelsDir := t.TempDir()
	internalPath := "onnxruntime-test/lib/libtest.so"

	archive := filepath.Join(dir, "runtime.tgz")
	makeTGZ(t, archive, []tgzEntry{{name: internalPath, content: testLibContent}})
	payload, err := os.ReadFile(archive)
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("tampered payload"))
	}))
	defer srv.Close()

	info := ortDownloadInfo{
		url:          srv.URL + "/onnxruntime-test.tgz",
		archiveName:  "runtime.tgz",
		libName:      "libtest.so",
		internalPath: internalPath,
		sha256:       sha256Hex(payload), // digest of the real archive, not the served bytes
	}
	err = installArchive(modelsDir, info, false, extractCPUArchive)
	if err == nil {
		t.Fatal("expected checksum error for a tampered archive")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("error = %q, want a checksum mismatch", err)
	}
	if entries := countDirEntries(t, modelsDir); entries != 0 {
		t.Fatalf("models dir has %d entries after a rejected archive, want 0", entries)
	}
}

func TestInstallArchive_verifiedDownloadExtractsAndCleansUp(t *testing.T) {
	dir := t.TempDir()
	modelsDir := t.TempDir()
	internalPath := "onnxruntime-test/lib/libtest.so"

	archive := filepath.Join(dir, "runtime.tgz")
	makeTGZ(t, archive, []tgzEntry{
		{name: internalPath, content: testLibContent},
		{name: "onnxruntime-test/README.md", content: "docs"},
	})
	payload, err := os.ReadFile(archive)
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	info := ortDownloadInfo{
		url:          srv.URL + "/onnxruntime-test.tgz",
		archiveName:  "runtime.tgz",
		libName:      "libtest.so",
		internalPath: internalPath,
		sha256:       sha256Hex(payload),
	}
	if err := installArchive(modelsDir, info, false, extractCPUArchive); err != nil {
		t.Fatalf("installArchive: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(modelsDir, "libtest.so"))
	if err != nil {
		t.Fatalf("extracted library missing: %v", err)
	}
	if string(got) != testLibContent {
		t.Fatalf("extracted content = %q, want %q", got, testLibContent)
	}
	for _, leftover := range []string{"runtime.tgz", "runtime.tgz.part", "runtime.tgz.sha256"} {
		if _, err := os.Stat(filepath.Join(modelsDir, leftover)); err == nil {
			t.Errorf("%s should be removed after a successful install", leftover)
		}
	}
}

func TestInstallArchive_gpuExtractor(t *testing.T) {
	dir := t.TempDir()
	modelsDir := t.TempDir()

	archive := filepath.Join(dir, "runtime-gpu.tgz")
	makeTGZ(t, archive, []tgzEntry{
		{name: "pkg/lib/libonnxruntime.so", content: "runtime"},
		{name: "pkg/lib/libonnxruntime_providers_cuda.so", content: "cuda"},
	})
	payload, err := os.ReadFile(archive)
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	info := ortDownloadInfo{
		url:         srv.URL + "/onnxruntime-gpu-test.tgz",
		archiveName: "runtime-gpu.tgz",
		libName:     "libonnxruntime_providers_cuda.so",
		sha256:      sha256Hex(payload),
	}
	if err := installArchive(modelsDir, info, false, extractGPUArchive); err != nil {
		t.Fatalf("installArchive: %v", err)
	}
	if got := countDirEntries(t, modelsDir); got != 2 {
		t.Fatalf("models dir has %d entries, want 2 provider libs", got)
	}
}
