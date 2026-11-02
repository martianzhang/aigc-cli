package agent

import (
	"strings"
	"testing"
)

// fakeLookPath builds an injectable lookPathFunc from a name→path map.
func fakeLookPath(found map[string]string) lookPathFunc {
	return func(name string) (string, bool) {
		p, ok := found[name]
		return p, ok
	}
}

func TestDetectShellFor(t *testing.T) {
	gitBash := `C:\Program Files\Git\bin\bash.exe`
	wslBash := `C:\Windows\System32\bash.exe`

	tests := []struct {
		name     string
		goos     string
		found    map[string]string
		wantKind shellKind
		wantPath string
		wantArgs []string
	}{
		{
			name:     "posix prefers zsh",
			goos:     "darwin",
			found:    map[string]string{"zsh": "/bin/zsh", "bash": "/bin/bash"},
			wantKind: shellPOSIX,
			wantPath: "/bin/zsh",
			wantArgs: []string{"-c"},
		},
		{
			name:     "posix falls back to bash",
			goos:     "linux",
			found:    map[string]string{"bash": "/bin/bash"},
			wantKind: shellPOSIX,
			wantPath: "/bin/bash",
			wantArgs: []string{"-c"},
		},
		{
			name:     "posix falls back to sh",
			goos:     "linux",
			found:    map[string]string{},
			wantKind: shellPOSIX,
			wantPath: "sh",
			wantArgs: []string{"-c"},
		},
		{
			name:     "windows prefers git bash on PATH",
			goos:     "windows",
			found:    map[string]string{"bash": gitBash, "powershell": `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`},
			wantKind: shellPOSIX,
			wantPath: gitBash,
			wantArgs: []string{"-c"},
		},
		{
			name:     "windows finds git bash at known location",
			goos:     "windows",
			found:    map[string]string{`C:\Program Files\Git\bin\bash.exe`: gitBash},
			wantKind: shellPOSIX,
			wantPath: gitBash,
			wantArgs: []string{"-c"},
		},
		{
			name:     "windows rejects wsl bash",
			goos:     "windows",
			found:    map[string]string{"bash": wslBash, "pwsh": `C:\Program Files\PowerShell\7\pwsh.exe`},
			wantKind: shellPowerShell,
			wantPath: `C:\Program Files\PowerShell\7\pwsh.exe`,
			wantArgs: []string{"-NoProfile", "-Command"},
		},
		{
			name:     "windows prefers busybox over powershell",
			goos:     "windows",
			found:    map[string]string{"busybox": `C:\tools\busybox.exe`, "powershell": `powershell.exe`},
			wantKind: shellPOSIX,
			wantPath: `C:\tools\busybox.exe`,
			wantArgs: []string{"sh", "-c"},
		},
		{
			name:     "windows falls back to powershell 5",
			goos:     "windows",
			found:    map[string]string{"powershell": `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`},
			wantKind: shellPowerShell,
			wantPath: `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`,
			wantArgs: []string{"-NoProfile", "-Command"},
		},
		{
			name:     "windows falls back to cmd",
			goos:     "windows",
			found:    map[string]string{},
			wantKind: shellCmd,
			wantPath: "cmd",
			wantArgs: []string{"/c"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectShellFor(tt.goos, fakeLookPath(tt.found))
			if got.kind != tt.wantKind {
				t.Errorf("kind = %v, want %v", got.kind, tt.wantKind)
			}
			if got.path != tt.wantPath {
				t.Errorf("path = %q, want %q", got.path, tt.wantPath)
			}
			if strings.Join(got.args, " ") != strings.Join(tt.wantArgs, " ") {
				t.Errorf("args = %v, want %v", got.args, tt.wantArgs)
			}
		})
	}
}

// TestWSLRunsAsLinux pins the WSL contract: a binary running inside WSL is a
// Linux build, so it must take the POSIX path and never the Windows fallback.
func TestWSLRunsAsLinux(t *testing.T) {
	// Even if Windows executables are visible through WSL interop, detection
	// only looks at the Linux names and picks the POSIX shell.
	spec := detectShellFor("linux", fakeLookPath(map[string]string{
		"bash":       "/bin/bash",
		"powershell": "/mnt/c/Windows/System32/WindowsPowerShell/v1.0/powershell.exe",
	}))
	if spec.kind != shellPOSIX {
		t.Fatalf("WSL shell kind = %v, want shellPOSIX", spec.kind)
	}
	if spec.path != "/bin/bash" {
		t.Errorf("WSL shell path = %q, want /bin/bash", spec.path)
	}
	if got := platformPromptFor("linux", spec); got != "" {
		t.Errorf("WSL PlatformPrompt = %q, want empty (Unix syntax works)", got)
	}
}

func TestIsWSLBash(t *testing.T) {
	for _, path := range []string{
		`C:\Windows\System32\bash.exe`,
		`c:\windows\system32\bash.exe`,
		`C:\Program Files\WindowsApps\Ubuntu\bash.exe`,
	} {
		if !isWSLBash(path) {
			t.Errorf("isWSLBash(%q) = false, want true", path)
		}
	}
	for _, path := range []string{
		`C:\Program Files\Git\bin\bash.exe`,
		`C:\msys64\usr\bin\bash.exe`,
		`/bin/bash`,
	} {
		if isWSLBash(path) {
			t.Errorf("isWSLBash(%q) = true, want false", path)
		}
	}
}

func TestNormalizeForPowerShell(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain command unchanged", "echo hello", "echo hello"},
		{"stdout to dev null", "echo hi > /dev/null", "echo hi >$null"},
		{"stderr to dev null", "cmd 2>/dev/null", "cmd 2>$null"},
		{"append to dev null", "cmd 2>>/dev/null", "cmd 2>>$null"},
		{"all streams to dev null", "cmd &>/dev/null", "cmd *>$null"},
		{"all streams append to dev null", "cmd &>>/dev/null", "cmd *>>$null"},
		{"stdin from dev null", "cat file < /dev/null", "cat file "},
		{"export", "export FOO=bar", `$env:FOO = "bar"`},
		{"which", "which python", "Get-Command python"},
		{"curl", "curl -s https://example.com", "curl.exe -s https://example.com"},
		{"touch", "touch a.txt", "New-Item -ItemType File -Force -Path a.txt | Out-Null"},
		{"rm -rf", "rm -rf build", "Remove-Item -Recurse -Force build"},
		{"rm -fr", "rm -fr build", "Remove-Item -Recurse -Force build"},
		{"rm -r -f", "rm -r -f build", "Remove-Item -Recurse -Force build"},
		{"mkdir -p", "mkdir -p out/nested", "New-Item -ItemType Directory -Force -Path out/nested"},
		{"cp -r", "cp -r src dst", "Copy-Item -Recurse src dst"},
		{"ls -la", "ls -la", "Get-ChildItem -Force"},
		{"ls -la with path", "ls -la src", "Get-ChildItem -Force -Path src"},
		{"ls -l", "ls -l", "Get-ChildItem"},
		{"ls -1", "ls -1", "Get-ChildItem | Select-Object -ExpandProperty Name"},
		{"ls with unsupported flag left alone", "ls -t", "ls -t"},
		{"find type and name", `find . -type f -name "*.go"`, `Get-ChildItem -Path . -Recurse -File -Filter "*.go"`},
		{"find type dir", "find . -type d", "Get-ChildItem -Path . -Recurse -Directory"},
		{"find name", `find src -name "*.md"`, `Get-ChildItem -Path src -Recurse -Filter "*.md"`},
		{"find name then type", `find . -name "*.go" -type f`, `Get-ChildItem -Path . -Recurse -File -Filter "*.go"`},
		{"find iname", `find . -iname "*.MD"`, `Get-ChildItem -Path . -Recurse -Filter "*.MD"`},
		{"find with other options left alone", "find . -maxdepth 2 -name x", "find . -maxdepth 2 -name x"},
		{"grep recursive", "grep -rn foo src", "Get-ChildItem -Path src -Recurse -File | Select-String -Pattern foo"},
		{"grep recursive cwd", "grep -rn foo", "Get-ChildItem -Path . -Recurse -File | Select-String -Pattern foo"},
		{"grep file", "grep -n foo file.txt", "Select-String -Path file.txt -Pattern foo"},
		{"grep plain", "grep foo file.txt", "Select-String -Path file.txt -Pattern foo"},
		{"grep unsupported flag left alone", "grep -l foo file.txt", "grep -l foo file.txt"},
		{"grep quoted pattern left alone", `grep -rn "hello world" src`, `grep -rn "hello world" src`},
		{"head -n", "head -n 5 file.txt", "Get-Content -Path file.txt -TotalCount 5"},
		{"head -N", "head -5 file.txt", "Get-Content -Path file.txt -TotalCount 5"},
		{"head bare", "head file.txt", "Get-Content -Path file.txt -TotalCount 10"},
		{"tail -n", "tail -n 5 file.txt", "Get-Content -Path file.txt -Tail 5"},
		{"tail -N", "tail -5 file.txt", "Get-Content -Path file.txt -Tail 5"},
		{"tail bare", "tail file.txt", "Get-Content -Path file.txt -Tail 10"},
		{"piped head left alone", "head -n 5 file.txt | grep x", "head -n 5 file.txt | grep x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeForPowerShell(tt.in); got != tt.want {
				t.Errorf("normalizeForPowerShell(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestNormalizeForCmd(t *testing.T) {
	if got := normalizeForCmd("cmd 2>/dev/null"); got != "cmd 2>NUL" {
		t.Errorf("normalizeForCmd = %q, want %q", got, "cmd 2>NUL")
	}
}

func TestPrepareCommand(t *testing.T) {
	posix := shellSpec{kind: shellPOSIX, path: "bash", args: []string{"-c"}}
	if got := prepareCommand(posix, "echo hi > /dev/null"); got != "echo hi > /dev/null" {
		t.Errorf("POSIX command was rewritten: %q", got)
	}

	pwsh := shellSpec{kind: shellPowerShell, path: "pwsh"}
	got := prepareCommand(pwsh, "echo hi > /dev/null")
	if !strings.HasPrefix(got, powerShellPrelude) {
		t.Errorf("PowerShell command missing UTF-8 prelude: %q", got)
	}
	if !strings.Contains(got, ">$null") {
		t.Errorf("PowerShell command not translated: %q", got)
	}

	cmd := shellSpec{kind: shellCmd, path: "cmd"}
	got = prepareCommand(cmd, "echo hi > /dev/null")
	if !strings.Contains(got, "chcp 65001") || !strings.Contains(got, ">NUL") {
		t.Errorf("cmd command not prepared: %q", got)
	}
}

func TestPlatformPromptFor(t *testing.T) {
	pwsh := shellSpec{kind: shellPowerShell, label: "Windows PowerShell 5.1"}
	if got := platformPromptFor("windows", pwsh); !strings.Contains(got, "$null") || !strings.Contains(got, "Select-String") {
		t.Errorf("PowerShell prompt missing guidance: %q", got)
	}

	gitBash := shellSpec{kind: shellPOSIX, label: "Git Bash / POSIX shell (bash -c)"}
	got := platformPromptFor("windows", gitBash)
	if !strings.Contains(got, "Unix syntax") || !strings.Contains(got, "Git Bash") {
		t.Errorf("Git Bash prompt missing guidance: %q", got)
	}
	if strings.Contains(got, "$null") {
		t.Errorf("Git Bash prompt should not warn about $null: %q", got)
	}

	if got := platformPromptFor("linux", gitBash); got != "" {
		t.Errorf("platformPromptFor(linux) = %q, want empty", got)
	}
}

func TestBashToolDescription(t *testing.T) {
	if got := bashToolDescription(); len(got) < 20 {
		t.Errorf("bashToolDescription too short: %q", got)
	}
}

func TestShellFailureHint(t *testing.T) {
	pwsh := shellSpec{kind: shellPowerShell, label: "PowerShell 7 (pwsh)"}
	if got := shellFailureHint(pwsh); !strings.Contains(got, "not a POSIX shell") || !strings.Contains(got, "read/find/grep/ls") {
		t.Errorf("PowerShell failure hint = %q", got)
	}
	if got := shellFailureHint(shellSpec{kind: shellPOSIX}); got != "" {
		t.Errorf("POSIX failure hint = %q, want empty", got)
	}
}
