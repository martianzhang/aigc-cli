# 主密钥与多机迁移

`aigc-cli` 会把配置文件里的 API Key 自动加密落盘。换电脑时，光复制 `config.yaml` 是不够的——还要决定是否把**主密钥**一起迁移过去。本文给出 macOS、Linux、Windows 的完整操作命令。

## 它保护什么

加密只作用于 `config.yaml` 中的敏感叶子字段：

- `api_key`、`api_secret`
- `providers.*.api_key`、`providers.*.api_secret`
- `web_search.*.api_key`

写法是 `enc:v1:<base64url(nonce||ciphertext)>`，算法为 AES-256-GCM，密钥由主密钥经 HKDF-SHA256（`config-v1` 域）派生。**其余配置（模型、代理、defaults 等）都是明文**，可以随意复制、同步、提交。

同一把主密钥还用于 `kb --vault` 的 age 保险箱，因此迁移主密钥也能带走保险箱数据。

## 主密钥存在哪里

主密钥是一个 age identity，形如 `AGE-SECRET-KEY-1...`。解析顺序固定为：

1. 环境变量 `AIGC_CLI_MASTER_KEY`（优先级最高）
2. 系统钥匙串：

| 平台 | 存储位置 | 定位方式 |
|---|---|---|
| macOS | 登录钥匙串 | service `aigc-cli-vault`，account `identity` |
| Linux | Secret Service（GNOME Keyring / KWallet） | 属性 `service=aigc-cli-vault`、`username=identity` |
| Windows | 凭据管理器（通用凭据） | target `aigc-cli-vault:identity`，user `identity` |

> 首次运行任意命令时若两处都没有密钥，CLI 会自动生成一把并写入钥匙串。钥匙串操作有 1 秒超时；不可用时只会提示设置 `AIGC_CLI_MASTER_KEY`，不会阻塞命令。`AIGC_CLI_NO_KEYRING` 非空则完全不访问钥匙串。

> 没有 `config key export/import` 之类的内建命令：`config get/set` 只读写 `config.yaml`，看不到主密钥。导出只能借助系统工具（下面给出命令）。

## 选择迁移策略

| | 方案 A：迁移主密钥 | 方案 B：每台机器独立密钥 |
|---|---|---|
| 适合 | 多机共用同一份 `config.yaml`；要带走 `kb --vault` 数据；不想逐台重填 | 一次性换机；更看重隔离性 |
| 操作量 | 导出/导入一次密钥 | 每台机器重新录入 API Key |
| 风险 | 密钥一旦泄露，所有机器全部沦陷 | 单机泄露不影响其他机器 |

下面两节分别给出步骤。

---

## 方案 A：迁移主密钥

### 1. 在旧机器导出

**macOS（zsh/bash）**

CLI 通过 `go-keyring` 写入 macOS 钥匙串时会加一层 `go-keyring-base64:` 前缀，因此 `-w` 拿到的可能不是最终值，需要判断并解码：

```bash
if RAW=$(security find-generic-password -s aigc-cli-vault -a identity -w 2>/dev/null); then
  case "$RAW" in
    go-keyring-base64:*)  printf '%s' "${RAW#go-keyring-base64:}"  | base64 -d ;;
    go-keyring-encoded:*) printf '%s' "${RAW#go-keyring-encoded:}" | xxd -r -p ;;
    *)                    printf '%s\n' "$RAW" ;;
  esac | tr -d '\r\n' > ~/aigc-master-key.txt
else
  echo "未找到钥匙串条目；如果你用的是环境变量，见下文" >&2
fi

chmod 600 ~/aigc-master-key.txt
head -c 14 ~/aigc-master-key.txt; echo    # 应输出 AGE-SECRET-KEY
```

> 较老的 macOS 上若 `base64 -d` 报错，改用 `base64 -D`。

**Linux（zsh/bash）**

```bash
secret-tool lookup service aigc-cli-vault username identity \
  | tr -d '\r\n' > ~/aigc-master-key.txt

chmod 600 ~/aigc-master-key.txt
head -c 14 ~/aigc-master-key.txt; echo    # 应输出 AGE-SECRET-KEY
```

需要 `libsecret-tools`（Debian/Ubuntu：`apt install libsecret-tools`；Fedora：`dnf install libsecret`）。集合被锁定时会弹出解锁对话框；无头环境会失败，此时只能在有桌面的会话里操作。

**Windows（PowerShell，读取凭据管理器）**

`cmdkey /list` 只能看条目、不能看密码，需要用 `CredRead`：

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

# 导出：把输出的值存进密码管理器
[AigcCred]::Read('aigc-cli-vault:identity')
```

**如果主密钥来自环境变量**

说明当初没有使用钥匙串（例如 CI 或手动设置），直接从环境读取即可：

```bash
# macOS / Linux
printf '%s\n' "$AIGC_CLI_MASTER_KEY"
```
```powershell
# Windows
$env:AIGC_CLI_MASTER_KEY
```

### 2. 安全传输

主密钥等同于「所有 API Key + 保险箱内容」的总钥匙：

- 优先用密码管理器（1Password / Bitwarden 的 Secure Note）或 `scp` 私网传输；
- 不要贴进聊天工具、邮件、issue、git；
- 传输完成后删除临时文件：`rm -f ~/aigc-master-key.txt`（Windows：`Remove-Item $HOME\aigc-master-key.txt`）。

例如私网内用 `scp`：

```bash
scp oldhost:~/aigc-master-key.txt ~/aigc-master-key.txt   # 在新机器上执行
```

### 3. 在新机器导入

**关键顺序：先让密钥可用，再运行任何 `aigc-cli` 命令。** 否则 CLI 会生成一把全新的密钥，之后再换回旧密钥时已加密内容会全部失效。

临时生效（只对当前 shell / 当前进程有效）：

```bash
# macOS / Linux
export AIGC_CLI_MASTER_KEY="$(cat ~/aigc-master-key.txt)"
```
```powershell
# Windows PowerShell
$env:AIGC_CLI_MASTER_KEY = Get-Content "$HOME\aigc-master-key.txt" -Raw
$env:AIGC_CLI_MASTER_KEY = $env:AIGC_CLI_MASTER_KEY.Trim()
```
```bat
:: Windows cmd（需要先手动把值复制进来，cmd 读取文件不方便）
set AIGC_CLI_MASTER_KEY=AGE-SECRET-KEY-1...
```

长期生效，任选一种：

```bash
# 写入 shell 配置（注意：这会把密钥明文存进 ~/.zshrc，权衡后再用）
echo 'export AIGC_CLI_MASTER_KEY="AGE-SECRET-KEY-1..."' >> ~/.zshrc

# 或写回系统钥匙串 —— macOS
security add-generic-password -U -s aigc-cli-vault -a identity -w "AGE-SECRET-KEY-1..."

# 或写回系统钥匙串 —— Linux（从 stdin 读取密钥）
printf '%s' 'AGE-SECRET-KEY-1...' | secret-tool store \
  --label="Password for 'identity' on 'aigc-cli-vault'" \
  service aigc-cli-vault username identity
```

```powershell
# Windows：写入用户级环境变量（对新开的终端生效）
setx AIGC_CLI_MASTER_KEY "AGE-SECRET-KEY-1..."
```
```powershell
# Windows：或写回凭据管理器（复用上面的 AigcCred 类）
[AigcCred]::Write('aigc-cli-vault:identity', 'identity', 'AGE-SECRET-KEY-1...')
```

写回钥匙串后即可 `unset AIGC_CLI_MASTER_KEY`（Windows：`Remove-Item Env:AIGC_CLI_MASTER_KEY`），CLI 会自动读取钥匙串。

> Windows 上不建议用 `cmdkey /generic:... /pass:...` 写入：它保存的 blob 编码与 CLI 读取时使用的 UTF-8 不一致，可能读出乱码。用 `setx` 或上面的 `CredWrite`。
>
> macOS 上用 `security add-generic-password -w` 写入原始字符串是兼容的：CLI 读取时只识别 `go-keyring-*` 前缀，没有前缀就按原样使用。

### 4. 迁移配置文件

```bash
# 路径在所有平台都是 <家目录>/.config/aigc-cli/config.yaml
scp ~/.config/aigc-cli/config.yaml 'newhost:~/.config/aigc-cli/'
```

若使用保险箱，再复制 `~/.config/aigc-cli/vault/`。`knowledge/`（索引库）和 `models/`（ONNX/TTS 模型）不必迁移，新机器上重新 `aigc-cli kb init` / `<命令> init` 即可。

### 5. 验证

```bash
aigc-cli --print-config     # 能正常输出且密钥为掩码 = 解密成功
aigc-cli config list        # 同上
aigc-cli config get api_key # 只显示掩码
```

如果报 `config decryption failed`，说明密钥不匹配：检查当前生效的是环境变量还是钥匙串，以及值是否完整（应以 `AGE-SECRET-KEY-` 开头、无换行、无引号）。

---

## 方案 B：不迁移主密钥

每台机器生成自己的主密钥，只迁移非敏感配置。

1. 在旧机器上生成一份去掉敏感字段的副本：

```bash
grep -vE '^[[:space:]]*(api_key|api_secret):' \
  ~/.config/aigc-cli/config.yaml > ~/config.sanitized.yaml
```

> 不要直接把带 `enc:v1:` 的配置复制到新机器：CLI 启动时解密失败会直接退出（exit 1），此时连 `config set` 都跑不了。

2. 把 `config.sanitized.yaml` 复制到新机器的 `~/.config/aigc-cli/config.yaml`。

3. 在新机器上首次运行，自动生成新主密钥，然后重新录入密钥：

```bash
aigc-cli config list
aigc-cli config set api_key 'sk-xxx' --force
aigc-cli config set providers.openrouter.api_key 'sk-yyy' --force
aigc-cli config set web_search.exa.api_key '...' --force
```

`config set` 写入时即加密，无需额外操作。

---

## 主密钥轮换

当前版本没有「重新加密全部配置」的命令。轮换主密钥的可行流程：

1. 把各 `api_key` / `api_secret` 的明文存进密码管理器（`config get` 只给掩码，无法导出明文）。
2. 换成新主密钥（环境变量或钥匙串）。
3. 删除 `config.yaml` 中所有 `enc:v1:` 行（或先按方案 B 的 `grep` 生成干净配置）。
4. 逐条 `config set ... --force` 重新写入。
5. `kb --vault` 中的旧数据无法用新密钥解密，需要先导出再重新加密（目前无自动重加密命令）。

## 常见问题

**Q：复制过去的 `config.yaml` 里全是 `enc:v1:`，新机器能直接用吗？**
A：只有在主密钥相同的情况下可以。否则会明确报错，不会静默忽略。

**Q：主密钥会进 shell history 吗？**
A：上面导出的命令把密钥写进文件或变量，argv 和 history 里都没有密钥。但 `security ... -w`、`secret-tool lookup` 会把值打印到终端，注意清理 scrollback；`setx AIGC_CLI_MASTER_KEY "..."` 会把密钥写进 cmd 的 history，建议改用 PowerShell 或先 `set /p`。

**Q：多台机器共用一把主密钥安全吗？**
A：这是便利性与爆炸半径的取舍。共用意味着任何一台机器或任何一次备份泄露都会波及全部；至少确保密钥只存在于密码管理器和本机钥匙串，不要写入 dotfiles 仓库。

**Q：钥匙串读不到密钥怎么办？**
A：确认条目名正确（macOS `aigc-cli-vault` / `identity`；Windows target 是 `aigc-cli-vault:identity`；Linux 属性是 `service` + `username`）。仍不行就用 `AIGC_CLI_MASTER_KEY` 直接注入，跨平台且不依赖钥匙串。

**Q：`config set AIGC_CLI_MASTER_KEY ...` 可以设置主密钥吗？**
A：不可以。它只会往 `config.yaml` 写一个 CLI 从不读取的普通字段，而且是明文（不会被自动加密），请勿这样操作。
