package agent

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// shellKind identifies the command language spoken by the shell the bash tool
// selects for this platform.
type shellKind int

const (
	// shellPOSIX is a Unix shell (zsh/bash/sh, or Git Bash/busybox on Windows).
	shellPOSIX shellKind = iota
	// shellPowerShell is Windows PowerShell 5.1 or PowerShell 7.
	shellPowerShell
	// shellCmd is the Windows command interpreter.
	shellCmd
)

// shellSpec is the resolved shell used to execute a bash tool call.
type shellSpec struct {
	kind  shellKind
	path  string
	args  []string
	label string
}

// Description returns a short human-readable shell label for prompts and tool
// descriptions, e.g. "Git Bash (bash -c)" or "Windows PowerShell 5.1".
func (s shellSpec) Description() string {
	if s.label != "" {
		return s.label
	}
	return filepath.Base(s.path)
}

// lookPathFunc mirrors exec.LookPath but is injectable for tests.
type lookPathFunc func(string) (string, bool)

func systemLookPath(name string) (string, bool) {
	p, err := exec.LookPath(name)
	if err != nil {
		return "", false
	}
	return p, true
}

// detectShell resolves the shell for the current process.
func detectShell() shellSpec {
	return detectShellFor(runtime.GOOS, systemLookPath)
}

// detectShellFor resolves the shell for goos using look. Windows prefers a
// real POSIX shell (Git Bash/MSYS2/Cygwin/busybox) so Unix commands, pipes and
// /dev/null keep working; only when none is installed does it fall back to
// PowerShell or cmd.exe.
func detectShellFor(goos string, look lookPathFunc) shellSpec {
	if goos != "windows" {
		if p, ok := look("zsh"); ok {
			return shellSpec{kind: shellPOSIX, path: p, args: []string{"-c"}, label: "zsh -c"}
		}
		if p, ok := look("bash"); ok {
			return shellSpec{kind: shellPOSIX, path: p, args: []string{"-c"}, label: "bash -c"}
		}
		return shellSpec{kind: shellPOSIX, path: "sh", args: []string{"-c"}, label: "sh -c"}
	}

	if p, ok := findWindowsPOSIXShell(look); ok {
		return shellSpec{kind: shellPOSIX, path: p, args: []string{"-c"}, label: "Git Bash / POSIX shell (" + filepath.Base(p) + " -c)"}
	}
	if p, ok := look("busybox"); ok {
		return shellSpec{kind: shellPOSIX, path: p, args: []string{"sh", "-c"}, label: "busybox sh -c"}
	}
	if p, ok := look("pwsh"); ok {
		return shellSpec{kind: shellPowerShell, path: p, args: []string{"-NoProfile", "-Command"}, label: "PowerShell 7 (pwsh)"}
	}
	if p, ok := look("powershell"); ok {
		return shellSpec{kind: shellPowerShell, path: p, args: []string{"-NoProfile", "-Command"}, label: "Windows PowerShell 5.1"}
	}
	return shellSpec{kind: shellCmd, path: "cmd", args: []string{"/c"}, label: "cmd.exe"}
}

// findWindowsPOSIXShell looks for a POSIX shell that shares the Windows
// filesystem view. WSL's bash.exe is deliberately rejected: it runs in a
// different namespace, so the inherited working directory and Windows paths do
// not map as the model expects.
//
// Note that running aigc-cli *inside* WSL is a different case: the binary is a
// Linux build, so runtime.GOOS is "linux" and detectShellFor never reaches this
// function. WSL users get the normal POSIX path with /dev/null and Unix
// utilities working natively. Only invoking a Windows aigc-cli.exe from WSL via
// interop takes the Windows branch (and then the cwd is a \\wsl$ UNC path).
func findWindowsPOSIXShell(look lookPathFunc) (string, bool) {
	if p, ok := look("bash"); ok && !isWSLBash(p) {
		return p, true
	}
	for _, candidate := range windowsPOSIXCandidates() {
		if p, ok := look(candidate); ok && !isWSLBash(p) {
			return p, true
		}
	}
	return "", false
}

// windowsPOSIXCandidates lists common Git-for-Windows / MSYS2 / Cygwin install
// locations that may not be on PATH.
func windowsPOSIXCandidates() []string {
	candidates := []string{
		`C:\Program Files\Git\bin\bash.exe`,
		`C:\Program Files\Git\usr\bin\bash.exe`,
		`C:\Program Files (x86)\Git\bin\bash.exe`,
		`C:\msys64\usr\bin\bash.exe`,
		`C:\cygwin64\bin\bash.exe`,
		`C:\cygwin\bin\bash.exe`,
	}
	for _, env := range []string{"LOCALAPPDATA", "USERPROFILE"} {
		if base := os.Getenv(env); base != "" {
			candidates = append(candidates,
				filepath.Join(base, `Programs\Git\bin\bash.exe`),
				filepath.Join(base, `scoop\apps\git\current\bin\bash.exe`),
			)
		}
	}
	return candidates
}

// isWSLBash reports whether path looks like the WSL launcher
// (C:\Windows\System32\bash.exe) rather than Git Bash.
func isWSLBash(path string) bool {
	p := strings.ToLower(strings.ReplaceAll(path, `\`, "/"))
	return strings.Contains(p, "/windows/system32/") || strings.Contains(p, "/windowsapps/")
}

// ----- Command preparation -----

const powerShellPrelude = "[Console]::OutputEncoding = [System.Text.Encoding]::UTF8; $OutputEncoding = [System.Text.Encoding]::UTF8; $ErrorActionPreference = 'Continue'; "

// prepareCommand adapts a model-authored command to the selected shell.
func prepareCommand(spec shellSpec, cmdLine string) string {
	switch spec.kind {
	case shellPowerShell:
		return powerShellPrelude + normalizeForPowerShell(cmdLine)
	case shellCmd:
		// cmd's default code page is GBK/CP936 on Chinese Windows.
		return "chcp 65001 >NUL & " + normalizeForCmd(cmdLine)
	default:
		return cmdLine
	}
}

// shellFailureHint is appended to a failed command's output so the model learns
// which shell it actually ran in and what to do differently.
func shellFailureHint(spec shellSpec) string {
	switch spec.kind {
	case shellPowerShell:
		return "\n[shell] The command ran through " + spec.Description() +
			", not a POSIX shell. Use PowerShell syntax ($null, Select-String, Get-ChildItem -Recurse) " +
			"or the built-in read/find/grep/ls tools."
	case shellCmd:
		return "\n[shell] The command ran through cmd.exe, not a POSIX shell. " +
			"Use NUL and cmd.exe syntax, or the built-in read/find/grep/ls tools."
	default:
		return ""
	}
}

var (
	rePowerShellAllAppend = regexp.MustCompile(`&>>\s*/dev/null`)
	rePowerShellAll       = regexp.MustCompile(`&>\s*/dev/null`)
	rePowerShellRedirect  = regexp.MustCompile(`([0-9]?>>|[0-9]?>)\s*/dev/null`)
	rePowerShellInput     = regexp.MustCompile(`<\s*/dev/null`)
	rePowerShellDevNull   = regexp.MustCompile(`/dev/null`)

	rePowerShellExport       = regexp.MustCompile(`^(\s*)export\s+([A-Za-z_][A-Za-z0-9_]*)=(.*)$`)
	rePowerShellWhich        = regexp.MustCompile(`^(\s*)which\s+(.+)$`)
	rePowerShellCurl         = regexp.MustCompile(`^(\s*)curl(\s+)`)
	rePowerShellTouch        = regexp.MustCompile(`^(\s*)touch\s+([^-]\S*.*)$`)
	rePowerShellRMRF         = regexp.MustCompile(`\brm\s+-(rf|fr)\b`)
	rePowerShellRMRFSeparate = regexp.MustCompile(`\brm\s+-r\s+-f\b`)
	rePowerShellRMRFReverse  = regexp.MustCompile(`\brm\s+-f\s+-r\b`)
	rePowerShellMkdir        = regexp.MustCompile(`\bmkdir\s+-p\b`)
	rePowerShellCpR          = regexp.MustCompile(`\bcp\s+-(r|R)\b`)

	rePowerShellHeadCount = regexp.MustCompile(`^(\s*)head\s+(-n\s+)?-?(\d+)\s+(.+)$`)
	rePowerShellTailCount = regexp.MustCompile(`^(\s*)tail\s+(-n\s+)?-?(\d+)\s+(.+)$`)
	rePowerShellHeadBare  = regexp.MustCompile(`^(\s*)head\s+([^-][^|>]*)$`)
	rePowerShellTailBare  = regexp.MustCompile(`^(\s*)tail\s+([^-][^|>]*)$`)

	rePowerShellFindTypeName = regexp.MustCompile(`^(\s*)find\s+(\S+)\s+-type\s+([fd])\s+-i?name\s+(\S+)\s*$`)
	rePowerShellFindNameType = regexp.MustCompile(`^(\s*)find\s+(\S+)\s+-i?name\s+(\S+)\s+-type\s+([fd])\s*$`)
	rePowerShellFindType     = regexp.MustCompile(`^(\s*)find\s+(\S+)\s+-type\s+([fd])\s*$`)
	rePowerShellFindName     = regexp.MustCompile(`^(\s*)find\s+(\S+)\s+-i?name\s+(\S+)\s*$`)

	rePowerShellGrepRecursive = regexp.MustCompile(`^(\s*)grep\s+-([a-zA-Z]*[rR][a-zA-Z]*)\s+(\S+)\s*(\S*)\s*$`)
	rePowerShellGrepFile      = regexp.MustCompile(`^(\s*)grep\s+-([a-zA-Z]+)\s+(\S+)\s+(\S+)\s*$`)
	rePowerShellGrepPlain     = regexp.MustCompile(`^(\s*)grep\s+(\S+)\s+(\S+)\s*$`)

	reCmdDevNull = regexp.MustCompile(`/dev/null`)
)

// normalizeForPowerShell translates the Unix idioms an agent most often emits
// into PowerShell equivalents. It intentionally covers only unambiguous forms;
// anything more involved is left alone (and the failure hint steers the model
// to the portable built-in tools).
func normalizeForPowerShell(cmd string) string {
	cmd = translateUnixLines(cmd)

	cmd = rePowerShellAllAppend.ReplaceAllString(cmd, "*>>$$null")
	cmd = rePowerShellAll.ReplaceAllString(cmd, "*>$$null")
	cmd = rePowerShellRedirect.ReplaceAllString(cmd, "${1}$$null")
	// PowerShell has no input redirection; reading from /dev/null is a no-op.
	cmd = rePowerShellInput.ReplaceAllString(cmd, "")
	cmd = rePowerShellDevNull.ReplaceAllString(cmd, "$$null")

	cmd = rePowerShellExport.ReplaceAllString(cmd, `${1}$$env:${2} = "${3}"`)
	cmd = rePowerShellWhich.ReplaceAllString(cmd, `${1}Get-Command ${2}`)
	cmd = rePowerShellCurl.ReplaceAllString(cmd, `${1}curl.exe${2}`)
	cmd = rePowerShellTouch.ReplaceAllString(cmd, `${1}New-Item -ItemType File -Force -Path ${2} | Out-Null`)
	cmd = rePowerShellRMRF.ReplaceAllString(cmd, "Remove-Item -Recurse -Force")
	cmd = rePowerShellRMRFSeparate.ReplaceAllString(cmd, "Remove-Item -Recurse -Force")
	cmd = rePowerShellRMRFReverse.ReplaceAllString(cmd, "Remove-Item -Recurse -Force")
	cmd = rePowerShellMkdir.ReplaceAllString(cmd, "New-Item -ItemType Directory -Force -Path")
	cmd = rePowerShellCpR.ReplaceAllString(cmd, "Copy-Item -Recurse")
	return cmd
}

// translateUnixLines rewrites simple Unix command invocations line by line.
// Lines with pipes/redirections/command separators are left untouched so a
// rewrite can never move the wrong tokens around. Complex or ambiguous forms
// also fall through to the real shell (and, on failure, the model gets the
// portable-tools hint).
func translateUnixLines(cmd string) string {
	lines := strings.Split(cmd, "\n")
	for i, line := range lines {
		line = translateLSLine(line)
		line = translateFindLine(line)
		line = translateGrepLine(line)
		lines[i] = translateHeadTailLine(line)
	}
	return strings.Join(lines, "\n")
}

// translateLSLine maps `ls -la`-style flags onto Get-ChildItem. Only flags that
// have a clear equivalent are accepted; anything else is left alone.
func translateLSLine(line string) string {
	if strings.ContainsAny(line, "|><;&") {
		return line
	}
	fields := strings.Fields(line)
	if len(fields) < 2 || fields[0] != "ls" {
		return line
	}

	force, recurse, onePerLine := false, false, false
	sawFlag := false
	var paths []string
	for _, f := range fields[1:] {
		if strings.HasPrefix(f, "-") && len(f) > 1 {
			flag := strings.TrimPrefix(f, "-")
			if strings.Trim(flag, "laRh1") != "" {
				return line
			}
			sawFlag = true
			force = force || strings.Contains(flag, "a")
			recurse = recurse || strings.Contains(flag, "R")
			onePerLine = onePerLine || strings.Contains(flag, "1")
			continue
		}
		paths = append(paths, f)
	}
	if !sawFlag {
		return line // PowerShell's built-in `ls` alias already handles it
	}

	out := "Get-ChildItem"
	if force {
		out += " -Force"
	}
	if recurse {
		out += " -Recurse"
	}
	if len(paths) > 0 {
		out += " -Path " + strings.Join(paths, " ")
	}
	if onePerLine {
		out += " | Select-Object -ExpandProperty Name"
	}
	return leadingWhitespace(line) + out
}

// translateFindLine maps the common `find <path> [-type f|d] [-name pat]`
// forms to Get-ChildItem. Any other find option is left untouched.
func translateFindLine(line string) string {
	if strings.ContainsAny(line, "|><;&") {
		return line
	}
	if m := rePowerShellFindTypeName.FindStringSubmatch(line); m != nil {
		return m[1] + "Get-ChildItem -Path " + m[2] + " -Recurse " + findKindFlag(m[3]) + " -Filter " + m[4]
	}
	if m := rePowerShellFindNameType.FindStringSubmatch(line); m != nil {
		return m[1] + "Get-ChildItem -Path " + m[2] + " -Recurse " + findKindFlag(m[4]) + " -Filter " + m[3]
	}
	if m := rePowerShellFindType.FindStringSubmatch(line); m != nil {
		return m[1] + "Get-ChildItem -Path " + m[2] + " -Recurse " + findKindFlag(m[3])
	}
	if m := rePowerShellFindName.FindStringSubmatch(line); m != nil {
		return m[1] + "Get-ChildItem -Path " + m[2] + " -Recurse -Filter " + m[3]
	}
	return line
}

func findKindFlag(kind string) string {
	if kind == "d" {
		return "-Directory"
	}
	return "-File"
}

// translateGrepLine maps simple `grep [-r] [-n] [-i] <pattern> [path]` forms to
// Select-String. Unsupported flags (e.g. -l, -w) leave the line untouched.
func translateGrepLine(line string) string {
	if strings.ContainsAny(line, "|><;&") {
		return line
	}
	if m := rePowerShellGrepRecursive.FindStringSubmatch(line); m != nil {
		if !onlyGrepFlags(m[2], "rRni") {
			return line
		}
		path := strings.TrimSpace(m[4])
		if path == "" {
			path = "."
		}
		return m[1] + "Get-ChildItem -Path " + path + " -Recurse -File | Select-String -Pattern " + m[3]
	}
	if m := rePowerShellGrepFile.FindStringSubmatch(line); m != nil {
		if !onlyGrepFlags(m[2], "rRni") || strings.ContainsAny(m[2], "rR") {
			return line
		}
		return m[1] + "Select-String -Path " + m[4] + " -Pattern " + m[3]
	}
	if m := rePowerShellGrepPlain.FindStringSubmatch(line); m != nil {
		return m[1] + "Select-String -Path " + m[3] + " -Pattern " + m[2]
	}
	return line
}

func onlyGrepFlags(flags, allowed string) bool {
	for _, r := range flags {
		if !strings.ContainsRune(allowed, r) {
			return false
		}
	}
	return true
}

func leadingWhitespace(s string) string {
	return s[:len(s)-len(strings.TrimLeft(s, " \t"))]
}

func translateHeadTailLine(line string) string {
	if strings.ContainsAny(line, "|><;&") {
		return line
	}
	if m := rePowerShellHeadCount.FindStringSubmatch(line); m != nil {
		return m[1] + "Get-Content -Path " + strings.TrimSpace(m[4]) + " -TotalCount " + m[3]
	}
	if m := rePowerShellTailCount.FindStringSubmatch(line); m != nil {
		return m[1] + "Get-Content -Path " + strings.TrimSpace(m[4]) + " -Tail " + m[3]
	}
	if m := rePowerShellHeadBare.FindStringSubmatch(line); m != nil {
		return m[1] + "Get-Content -Path " + strings.TrimSpace(m[2]) + " -TotalCount 10"
	}
	if m := rePowerShellTailBare.FindStringSubmatch(line); m != nil {
		return m[1] + "Get-Content -Path " + strings.TrimSpace(m[2]) + " -Tail 10"
	}
	return line
}

// normalizeForCmd translates /dev/null for cmd.exe, whose null device is NUL.
func normalizeForCmd(cmd string) string {
	return reCmdDevNull.ReplaceAllString(cmd, "NUL")
}

// ----- Prompt / tool-description text -----

// PlatformPrompt returns platform-specific shell guidance for the chat system
// prompt. It is empty on platforms that already use a POSIX shell.
func PlatformPrompt() string {
	return platformPromptFor(runtime.GOOS, detectShell())
}

func platformPromptFor(goos string, spec shellSpec) string {
	if goos != "windows" {
		return ""
	}
	const preferTools = "For reading and searching files, prefer the built-in read/find/grep/ls tools — they are portable Go implementations."
	if spec.kind == shellPOSIX {
		return "Runtime environment: Windows. The bash tool runs commands through " + spec.Description() +
			", so Unix syntax, pipes and /dev/null work. Use POSIX-style paths (relative or /c/...) rather than Windows backslash paths. " +
			preferTools
	}
	if spec.kind == shellPowerShell {
		return "Runtime environment: Windows. The bash tool does NOT run a POSIX shell: commands go through " + spec.Description() +
			". Unix commands such as /dev/null, grep, find, sed, awk, head and tail do not work as written. " +
			"Use $null instead of /dev/null, Select-String for text search, and Get-ChildItem -Recurse for recursive file listing. " +
			"It translates a few common idioms automatically, but do not rely on it. " +
			preferTools
	}
	return "Runtime environment: Windows. The bash tool runs commands through cmd.exe, not a POSIX shell. " +
		"Use NUL instead of /dev/null and cmd.exe syntax; Unix commands such as ls, grep, find, sed and awk are unavailable. " +
		preferTools
}

// bashToolDescription is the LLM-facing description of the bash primitive. It
// names the actual shell so the model emits syntax that will run.
func bashToolDescription() string {
	spec := detectShell()
	base := "Execute a shell command in the current working directory and return its combined stdout/stderr. Prefer the dedicated read/find/grep/ls tools for file operations; use bash for other CLI work. Pass `timeout` (seconds) to override the default limit (300s unless defaults.chat.bash.timeout is configured). Requires `tools_enable` to list \"bash\"."
	if spec.kind == shellPOSIX {
		return base + " Commands run through " + spec.Description() + " (Unix syntax: pipes, /dev/null, ls, grep, find)."
	}
	return base + " Commands run through " + spec.Description() + ", NOT a POSIX shell — use its native syntax ($null instead of /dev/null, PowerShell cmdlets instead of Unix utilities)."
}
