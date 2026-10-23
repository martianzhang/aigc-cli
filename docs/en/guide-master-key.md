# Master Key & Multi-Machine Migration

`aigc-cli` encrypts API keys in `config.yaml` at rest. When you move to another computer, copying `config.yaml` alone is not enough — you must also decide whether to migrate the **master key**. This guide gives complete commands for macOS, Linux, and Windows.

## What it protects

Only sensitive leaf fields in `config.yaml` are encrypted:

- `api_key`, `api_secret`
- `providers.*.api_key`, `providers.*.api_secret`
- `web_search.*.api_key`

Values are written as `enc:v1:<base64url(nonce||ciphertext)>` using AES-256-GCM, with the key derived from the master key via HKDF-SHA256 (domain `config-v1`). **Everything else (models, proxy, defaults, …) is plaintext** and safe to copy or sync.

The same master key also backs the `kb --vault` age vault, so migrating it brings vault data along too.

## Where the master key lives

The master key is an age identity of the form `AGE-SECRET-KEY-1...`. Resolution order:

1. `AIGC_CLI_MASTER_KEY` environment variable (highest priority)
2. The system keyring:

| Platform | Location | Coordinates |
|---|---|---|
| macOS | Login keychain | service `aigc-cli-vault`, account `identity` |
| Linux | Secret Service (GNOME Keyring / KWallet) | attributes `service=aigc-cli-vault`, `username=identity` |
| Windows | Credential Manager (generic credential) | target `aigc-cli-vault:identity`, user `identity` |

> On the first run of any command, if neither location holds a key, the CLI generates one and stores it in the keyring. Keyring calls time out after 1 second; if unavailable, the CLI only prints a hint to set `AIGC_CLI_MASTER_KEY` and never blocks. A non-empty `AIGC_CLI_NO_KEYRING` disables keyring access entirely.

> There is no built-in `config key export/import`: `config get/set` only read/write `config.yaml` and never expose the master key. Exporting therefore uses the OS tools below.

## Choosing a strategy

| | Option A: migrate the master key | Option B: per-machine keys |
|---|---|---|
| Good for | Sharing one `config.yaml` across machines; keeping `kb --vault` data; avoiding re-entry | One-off machine moves; stronger isolation |
| Effort | Export/import the key once | Re-enter API keys per machine |
| Risk | One leaked key compromises every machine | A single machine leak stays contained |

---

## Option A: migrate the master key

### 1. Export on the old machine

**macOS (zsh/bash)**

`go-keyring` wraps values it writes to the macOS keychain with a `go-keyring-base64:` prefix, so `-w` may not return the final value. Detect and decode:

```bash
if RAW=$(security find-generic-password -s aigc-cli-vault -a identity -w 2>/dev/null); then
  case "$RAW" in
    go-keyring-base64:*)  printf '%s' "${RAW#go-keyring-base64:}"  | base64 -d ;;
    go-keyring-encoded:*) printf '%s' "${RAW#go-keyring-encoded:}" | xxd -r -p ;;
    *)                    printf '%s\n' "$RAW" ;;
  esac | tr -d '\r\n' > ~/aigc-master-key.txt
else
  echo "keychain item not found; see the env-var case below" >&2
fi

chmod 600 ~/aigc-master-key.txt
head -c 14 ~/aigc-master-key.txt; echo    # should print AGE-SECRET-KEY
```

> On older macOS releases where `base64 -d` fails, use `base64 -D`.

**Linux (zsh/bash)**

```bash
secret-tool lookup service aigc-cli-vault username identity \
  | tr -d '\r\n' > ~/aigc-master-key.txt

chmod 600 ~/aigc-master-key.txt
head -c 14 ~/aigc-master-key.txt; echo    # should print AGE-SECRET-KEY
```

Requires `libsecret-tools` (`apt install libsecret-tools` on Debian/Ubuntu, `dnf install libsecret` on Fedora). A locked collection triggers an unlock prompt; headless sessions fail, so run this from a desktop session.

**Windows (PowerShell, reading Credential Manager)**

`cmdkey /list` shows the entry but never the password, so use `CredRead`:

```powershell
Add-Type -TypeDefinition @"
using System;
using System.ComponentModel;
using System.Runtime.InteropServices;
using System.Text;

public static class AigcCred {
    [StructLayout(LayoutKind.Sequential, CharSet = CharSet.Unicode)]
    private struct CREDENTIAL {
        public uint Flags;
        public uint Type;
        public string TargetName;
        public string Comment;
        public System.Runtime.InteropServices.ComTypes.FILETIME LastWritten;
        public uint CredentialBlobSize;
        public IntPtr CredentialBlob;
        public uint Persist;
        public uint AttributeCount;
        public IntPtr Attributes;
        public string TargetAlias;
        public string UserName;
    }

    [DllImport("advapi32.dll", CharSet = CharSet.Unicode, SetLastError = true)]
    private static extern bool CredReadW(string target, uint type, uint flags, out IntPtr credential);

    [DllImport("advapi32.dll", CharSet = CharSet.Unicode, SetLastError = true)]
    private static extern bool CredWriteW(ref CREDENTIAL credential, uint flags);

    [DllImport("advapi32.dll")]
    private static extern void CredFree(IntPtr buffer);

    public static string Read(string target) {
        IntPtr p;
        if (!CredReadW(target, 1, 0, out p))
            throw new Win32Exception(Marshal.GetLastWin32Error());
        try {
            var c = (CREDENTIAL)Marshal.PtrToStructure(p, typeof(CREDENTIAL));
            var b = new byte[c.CredentialBlobSize];
            Marshal.Copy(c.CredentialBlob, b, 0, b.Length);
            return Encoding.UTF8.GetString(b);
        } finally { CredFree(p); }
    }

    public static void Write(string target, string user, string secret) {
        var c = new CREDENTIAL();
        c.Type = 1;        // CRED_TYPE_GENERIC
        c.Persist = 2;     // CRED_PERSIST_LOCAL_MACHINE
        c.TargetName = target;
        c.UserName = user;
        var b = Encoding.UTF8.GetBytes(secret);
        c.CredentialBlobSize = (uint)b.Length;
        c.CredentialBlob = Marshal.AllocHGlobal(b.Length);
        try {
            Marshal.Copy(b, 0, c.CredentialBlob, b.Length);
            if (!CredWriteW(ref c, 0))
                throw new Win32Exception(Marshal.GetLastWin32Error());
        } finally { Marshal.FreeHGlobal(c.CredentialBlob); }
    }
}
"@

# Export: store the printed value in your password manager
[AigcCred]::Read('aigc-cli-vault:identity')
```

**If the key comes from an environment variable**

That means the keyring was never used (CI, manual setup). Read it directly:

```bash
# macOS / Linux
printf '%s\n' "$AIGC_CLI_MASTER_KEY"
```
```powershell
# Windows
$env:AIGC_CLI_MASTER_KEY
```

### 2. Transfer it securely

The master key is the root key for every API key and the whole vault:

- Prefer a password manager (1Password / Bitwarden secure note) or `scp` over a private network;
- Never paste it into chat, email, issues, or git;
- Remove the temp file afterwards: `rm -f ~/aigc-master-key.txt` (Windows: `Remove-Item $HOME\aigc-master-key.txt`).

For example, over a private network with `scp`:

```bash
scp oldhost:~/aigc-master-key.txt ~/aigc-master-key.txt   # run on the new machine
```

### 3. Import on the new machine

**Order matters: make the key available before running any `aigc-cli` command.** Otherwise the CLI generates a brand-new key, and switching back later leaves all previously encrypted values unreadable.

Temporary (current shell / process only):

```bash
# macOS / Linux
export AIGC_CLI_MASTER_KEY="$(cat ~/aigc-master-key.txt)"
```
```powershell
# Windows PowerShell
$env:AIGC_CLI_MASTER_KEY = (Get-Content "$HOME\aigc-master-key.txt" -Raw).Trim()
```
```bat
:: Windows cmd (paste the value manually; cmd cannot easily read a file)
set AIGC_CLI_MASTER_KEY=AGE-SECRET-KEY-1...
```

Persistent, pick one:

```bash
# Shell profile (note: stores the key in plaintext in ~/.zshrc — weigh the trade-off)
echo 'export AIGC_CLI_MASTER_KEY="AGE-SECRET-KEY-1..."' >> ~/.zshrc

# Or write it back to the system keyring — macOS
security add-generic-password -U -s aigc-cli-vault -a identity -w "AGE-SECRET-KEY-1..."

# Or keyring — Linux (reads the secret from stdin)
printf '%s' 'AGE-SECRET-KEY-1...' | secret-tool store \
  --label="Password for 'identity' on 'aigc-cli-vault'" \
  service aigc-cli-vault username identity
```

```powershell
# Windows: user-level environment variable (new terminals only)
setx AIGC_CLI_MASTER_KEY "AGE-SECRET-KEY-1..."
```
```powershell
# Windows: or write it back to Credential Manager (reuse the AigcCred class above)
[AigcCred]::Write('aigc-cli-vault:identity', 'identity', 'AGE-SECRET-KEY-1...')
```

After writing to the keyring, `unset AIGC_CLI_MASTER_KEY` (Windows: `Remove-Item Env:AIGC_CLI_MASTER_KEY`); the CLI reads the keyring automatically.

> Do not use `cmdkey /generic:... /pass:...` on Windows: its stored blob encoding does not match the UTF-8 the CLI reads, so the value may come back garbled. Use `setx` or `CredWrite` above.
>
> Writing the raw string via `security add-generic-password -w` on macOS is compatible: the CLI only decodes the `go-keyring-*` prefixes and otherwise uses the value verbatim.

### 4. Move the config file

```bash
# The path is <home>/.config/aigc-cli/config.yaml on every platform
scp ~/.config/aigc-cli/config.yaml 'newhost:~/.config/aigc-cli/'
```

Also copy `~/.config/aigc-cli/vault/` if you use the vault. `knowledge/` (indexes) and `models/` (ONNX/TTS) do not need migration — re-run `aigc-cli kb init` / `<command> init` on the new machine.

### 5. Verify

```bash
aigc-cli --print-config     # prints normally with masked keys = decryption works
aigc-cli config list        # same
aigc-cli config get api_key # masked only
```

If you see `config decryption failed`, the keys do not match: check whether the env var or the keyring is winning, and that the value is intact (starts with `AGE-SECRET-KEY-`, no newline, no quotes).

---

## Option B: no key migration

Each machine generates its own master key; only non-sensitive settings move.

1. On the old machine, produce a copy without secrets:

```bash
grep -vE '^[[:space:]]*(api_key|api_secret):' \
  ~/.config/aigc-cli/config.yaml > ~/config.sanitized.yaml
```

> Do not copy a config containing `enc:v1:` to a machine with a different key: startup decryption fails and the CLI exits (code 1) — even `config set` cannot run then.

2. Copy `config.sanitized.yaml` to `~/.config/aigc-cli/config.yaml` on the new machine.

3. Run once to generate a new master key, then re-enter the keys:

```bash
aigc-cli config list
aigc-cli config set api_key 'sk-xxx' --force
aigc-cli config set providers.openrouter.api_key 'sk-yyy' --force
aigc-cli config set web_search.exa.api_key '...' --force
```

`config set` encrypts on write; nothing else to do.

---

## Rotating the master key

There is no "re-encrypt everything" command today. A workable rotation:

1. Save every `api_key` / `api_secret` plaintext in a password manager (`config get` only shows masks, so it cannot export them).
2. Switch to the new master key (env var or keyring).
3. Remove all `enc:v1:` lines from `config.yaml` (or use the `grep` from Option B to produce a clean file).
4. Re-add each secret with `config set ... --force`.
5. Old `kb --vault` data cannot be decrypted with the new key; export and re-encrypt it (no automatic re-encryption command yet).

## FAQ

**Q: The copied `config.yaml` is full of `enc:v1:` — will it just work?**
A: Only with the same master key. Otherwise the CLI fails loudly; it never ignores undecryptable values.

**Q: Does the master key end up in shell history?**
A: The export commands write to a file or variable, so neither argv nor history contains the key. However `security ... -w` and `secret-tool lookup` print it to the terminal — clear your scrollback. `setx AIGC_CLI_MASTER_KEY "..."` does put the key into cmd history; prefer PowerShell or `set /p`.

**Q: Is sharing one master key across machines safe?**
A: It is a trade-off between convenience and blast radius. Sharing means any machine or backup leak affects everything; at minimum keep the key only in a password manager and local keyrings, never in a dotfiles repo.

**Q: The keyring lookup fails — what now?**
A: Verify the item names (macOS `aigc-cli-vault` / `identity`; Windows target `aigc-cli-vault:identity`; Linux attributes `service` + `username`). If it still fails, inject `AIGC_CLI_MASTER_KEY` directly — cross-platform and keyring-independent.

**Q: Can `config set AIGC_CLI_MASTER_KEY ...` set the master key?**
A: No. It writes an ordinary, never-read field into `config.yaml` — in plaintext, since it is not on the auto-encryption list. Don't do it.
