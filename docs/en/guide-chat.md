# AI Chat

Use `aigc-cli chat` for interactive multi-turn AI conversations with tool-calling support.

## Basic Usage

```bash
# Single message
aigc-cli chat --message "Hello, who are you?"

# Interactive REPL mode
aigc-cli chat
```

## Interactive REPL

Start the interactive chat:

```bash
aigc-cli chat
> Hello
> generate an image of a cat
```

The REPL supports:
- Multi-turn conversation
- Tool calling (built-in primitives + tools from configured MCP servers)
- Streaming output
- Command history
- Scroll the conversation with PgUp/PgDn or the mouse wheel (to select text, hold the terminal's mouse-reporting bypass key while dragging — `Shift` on most terminals, **`Option`/`⌥` on iTerm2**; `/copy` copies the last assistant response)
- Slash commands, and `!<shell_cmd>` to run a shell command (the message area echoes `$ <cmd>` followed by its output)
- `@path` file references: text files are inlined, images are attached (Tab completes paths; governed by `defaults.chat.read_deny`)

## File References (@)

Reference local files in a message with `@path` — no need to paste their contents.

```bash
# Interactive: type @ and press Tab to complete a path
aigc-cli chat
> review @internal/cli/chat/chat.go

# Works non-interactively too
aigc-cli chat --message "explain @main.go"
```

| File type | Behaviour |
|---|---|
| Text file | Inlined in place as `<file name="path">content</file>` |
| Image (png / jpeg / gif / webp) | Sent as an image attachment (base64); the text keeps an `[attached image: path]` marker. Requires a vision-capable model |
| Binary file | Not inlined; the `@path` token is left as-is with a warning |
| Text > 1 MiB / image > 20 MiB | Refused with a warning |
| Unresolvable token | Left as-is (`@user` and `@scope/pkg` are never mistaken for paths) |

**Tab completion:** type `@` then press `Tab` to cycle matching project files. Inside a git repository this uses `git ls-files` (honouring `.gitignore`); otherwise it falls back to a walk that skips hidden entries.

**Safety:**

- `@` reads share the exact policy of the `read` tool: patterns in `defaults.chat.read_deny` block them the same way. A blocked file is never read — only a warning is shown.
- Binary files are never inlined as text, so garbled bytes cannot be pushed into the context.
- Applies to **user messages only**; `@` inside tool-call arguments is expanded by separate logic (see the `read` primitive below).

## Tool Calling

Chat ships a small set of **primitives** (names aligned with pi coding agent). Domain capabilities (generation, detection, knowledge base, …) are provided by **MCP servers** instead (see [MCP servers](#mcp-servers-chat-as-an-mcp-client)).

| Primitive | Default | Description |
|---|---|---|
| `read` | ✅ | Read a local text file (paginated via offset/limit) |
| `grep` | ✅ | Regex search file contents |
| `find` | ✅ | Find files by name/glob (recursive) |
| `ls` | ✅ | List a directory (non-recursive) |
| `write` | ⛔ | Write/overwrite a file, creating parent dirs |
| `edit` | ⛔ | Exact-text replace (`old_string` must be unique) |
| `bash` | ⛔ | Execute a shell command in the current directory |
| `search` | ⛔ | Built-in web search (DuckDuckGo, free, no key); returns titles + URLs |
| `fetch` | ⛔ | Built-in URL fetch: HTML → Markdown (main content, token-efficient), non-HTML as-is; free, SSRF-guarded |
| `web_search` | ⛔ | Search through a **third-party API** (vendor chosen by `search_provider`; may be billed); returns titles + URLs |
| `web_fetch` | ⛔ | Fetch through a **third-party API** (same vendor; returns provider-formatted Markdown; may be billed) |

**Read-only primitives** (`read`/`grep`/`find`/`ls`) are on by default. **Privileged primitives** (`write`/`edit`/`bash`/`search`/`fetch`/`web_search`/`web_fetch`) are off by default and require an explicit `tools_enable` entry — the safety gate for handing file-writes/command execution/network access to the model.

Domain tools (`generate_image`, `recognize_text`, `kb_find`, …) are no longer defined in chat; they come from MCP servers. To use aigc-cli's own tools, point `mcp.servers` at `aigc-cli mcp` (see below).

> `read` can read **any file** by default. To block sensitive files, set `defaults.chat.read_deny` to a list of **filename/path regexes** (Go regexp, matched against the path or base name); leave it empty to allow everything. Example: `read_deny: ["\\.pem$", "^id_rsa$", "\\.env$"]`.
>
> **Binary protection**: `read` sniffs content — files with NUL bytes or many invalid UTF-8 sequences (images, executables, …) are not returned as text; they report `[binary file — not displayed as text]` so garbled bytes never enter the context.
>
> **`fetch` main-content extraction**: HTML fetched by the built-in `fetch` is first run through readability to extract the main article, then converted to Markdown (nav/footer boilerplate dropped — a big token saving); when no article can be extracted it falls back to converting the whole page. Non-HTML (JSON, plain text, …) is returned as-is. Results are paginated by **character** (not byte): `max_length` defaults to 5000 and caps at 50000; read further with the `offset` given in the response.
>
> The built-in `fetch` is free and **SSRF-guarded**: before the request it rejects loopback / private / link-local / cloud-metadata addresses (e.g. `169.254.169.254`) and re-validates every redirect hop. The third-party `web_fetch` sends the request to the vendor, whose own security policy applies.

### Web access: built-in vs third-party

Chat has two sets of web primitives. The naming rule is — **no `web_` prefix = built-in and free; a `web_` prefix = goes out to a third-party API (may be billed)**:

| Capability | Built-in (free) | Third-party API (billed by the vendor) |
|---|---|---|
| Search | `search` (DuckDuckGo scrape) | `web_search` (vendor chosen by `search_provider`) |
| Fetch | `fetch` (local HTTP + readability) | `web_fetch` (same vendor's fetch endpoint) |

All four are **privileged primitives** and must be listed in `tools_enable`:

```yaml
tools_enable:
  - "search"
  - "fetch"
  - "web_search"
  - "web_fetch"
```

The built-in `search`/`fetch` need **no configuration and cost nothing**. The third-party `web_search`/`web_fetch` are served by the vendors declared under the top-level `web_search.*`, with `defaults.chat.search_provider` selecting the **vendor**. Vendors generally offer both search and fetch, so the two primitives **share one vendor selection** — there is no separate fetch setting:

```yaml
web_search:
  deepseek:
    type: deepseek             # in-house engine, provides both Search and Fetch
    api_key: "sk-xxx"          # $0.75/1000 successful requests (Search and Fetch alike)
  doubao:
    type: doubao
    api_key: "your-doubao-api-key"
  firecrawl:
    type: firecrawl
    api_key: "fc-xxx"
    tags: [free, quality]

defaults:
  chat:
    search_provider: deepseek   # duckduckgo/firecrawl/brave/doubao/deepseek, or auto/free/quality (default auto)
```

- `search_provider`: a vendor name forces that vendor; `auto` (default) picks by `free`/`quality` tags; `free`/`cheap` uses only free engines; `quality` uses only quality engines.
- The `--search-provider` flag overrides the config for one run (precedence: `--search-provider` > `defaults.chat.search_provider` > `auto`).
- `web_search` uses **third-party vendors only**: the built-in DuckDuckGo is excluded, so `auto` can never silently answer from a non-third-party engine. With no third-party vendor declared, `web_search`/`web_fetch` **error out and point you at the built-in `search`/`fetch`** — no silent spend, no silent downgrade.
- `web_fetch` needs a **fetch-capable vendor** (e.g. `deepseek`); search-only vendors (`duckduckgo`/`brave`) do not provide it.
- One call returns 5 results by default, at most 10.
- Unlike `kb search`, chat's web primitives **search/fetch only — they do not save to the knowledge base**; `kb search` still uses the same `web_search.*` vendor config (including the free DuckDuckGo fallback).

### Project context (AGENTS.md)

On startup, Chat loads `AGENTS.md` as **project context** and appends it to the system prompt:

- Global: `~/.config/aigc-cli/AGENTS.md` (applies to every session);
- Project: it searches `AGENTS.md` **upward** from the current directory to the repository root (the first ancestor containing `.git`); multiple files are concatenated outermost → innermost (innermost is most specific).

Nothing is added when no file exists. Put project conventions, build/test commands, and style notes here.

Tools can be enabled/disabled via config:

```yaml
# Top-level: opt into privileged primitives (read-only ones are on by default)
tools_enable:
  - "read"
  - "grep"
  - "find"
  - "bash"        # enable shell
  - "write"       # enable file writes
  - "web_search"  # third-party search API (billed by the vendor)
  - "web_fetch"   # third-party fetch API (billed by the vendor)
  # - "search"    # built-in web search (DuckDuckGo; free, no key)
  # - "fetch"     # built-in fetch (local readability; free)

# Top-level blocklist always wins
tools_disable:
  - "remove_watermark"

defaults:
  chat:
    tools: ["*"]              # narrow the (primitives ∪ MCP) union; empty or ["*"] = all
    # disable_tools: ["grep"]
```

### MCP servers (chat as an MCP client)

Chat can act as an **MCP client**: it connects to arbitrary stdio MCP servers and
merges their tools into the agent's toolset. There is **no built-in server** — to
use aigc-cli's own AIGC tools from chat, add an entry pointing at `aigc-cli mcp`:

```yaml
mcp:
  servers:
    # aigc-cli's own AIGC tools
    - name: aigc
      command: aigc-cli                    # use an absolute path if not on $PATH
      args: ["mcp"]
      # tools_enable: ["generate_*"]        # expose only matching tools (glob, empty = all)
      # tools_disable: ["remove_watermark"] # exclude tools (wins over enable)
      # enabled: true                       # default true
      # env: ["KEY=VALUE"]                  # extra env for the child process
    # third-party example
    - name: playwright
      command: npx
      args: ["-y", "@playwright/mcp"]
      tools_enable: ["browser_*"]
```

| Field | Description |
|---|---|
| `name` | Server id (logs/errors); falls back to `command` when empty |
| `command` / `args` | Launch command (currently **stdio** only) |
| `env` | Extra/overriding child env vars (`KEY=VALUE`); parent env is inherited |
| `tools_enable` / `tools_disable` | Filter tools exposed to the model from this server (glob; disable wins) — **chat-side only** |
| `enabled` | Whether to connect (default `true`) |

- Per-server `tools_enable`/`tools_disable` only affect **what chat sees**; they do not change the MCP server's own exposed tool surface (controlled by the top-level `tools_enable`/`tools_disable`).
- **Built-in primitives win over same-named MCP tools**: if a server also exposes `search`/`fetch`/`read`/`bash`/…, chat always uses **the CLI's own implementation** — the colliding MCP tool is neither sent to the model nor executed, and `-v` logs the names that were shadowed. This also avoids emitting two functions with the same name, which most providers reject. Rename the MCP tool if you need its version to take effect.
- The merged toolset is still subject to the top-level `tools_enable`/`tools_disable` and `defaults.chat.tools`/`disable_tools`.
- A server that fails to connect (missing command, init timeout, …) is **skipped** with a stderr notice; other servers and the chat keep working.
- Connections open at startup and close on exit; `/tools` lists every tool, including MCP-provided ones.
- Image/audio tool results are surfaced to the model as **local file paths** (no inlined media); generated files are already on disk, so the model reports their locations.

### On-demand tool loading (optional)

By default every tool from every connected MCP server is sent with each request (its schema stays in context). When you have many tools and context is tight, enable on-demand loading:

```yaml
defaults:
  chat:
    tools_on_demand: true
```

With it on, a request carries only the **primitives** plus two meta-tools: the model calls `search_tools` (keyword → tool names + one-line descriptions) to find what it needs, then `load_tools` to enable them — the full schemas enter context on **subsequent** turns. Primitives are always available and unaffected.

## Parameters

| Parameter | Description | Default |
|---|---|---|
| `--message` / `-m` | Single message (non-interactive) | — |
| `--model` | Model name | `deepseek-v4-flash` |
| `--temperature` | Response temperature | `0.7` |
| `--max-output` | Max response tokens | `4096` |
| `--context-size` | Max input context tokens; auto-compacts at 80% (summarizes older messages) | `0` (model default) |
| `--system` | System prompt | — |
| `--json` | Pass messages as JSON; forwarded **verbatim** when no flags are set, so unmodeled fields (e.g. `reasoning_effort`, vendor-private keys) reach the API. Explicitly-set flags override the matching key | — |
| `--inject-json` | Merge a JSON object **on top of** the generated body (objects recurse, arrays append, scalars overwrite). Use it for vendor-only fields (native web search, reasoning, routing); takes precedence over `--json` | — |
| `--dry-run` | Print the equivalent curl (real endpoint + body) without calling the API | — |
| `--search-provider` | Vendor for the `web_search`/`web_fetch` primitives (`duckduckgo`/`firecrawl`/`brave`/`doubao`/`deepseek`, or `auto`/`free`/`quality`); overrides `defaults.chat.search_provider` | — |

### Context Management

When `--context-size` is set, the conversation history is automatically summarized
(compacted) once it exceeds 80% of the limit, freeing space without manual action.
You can also type `/compact` in interactive mode to trigger compaction manually.

## JSON Mode

```bash
aigc-cli chat --json '{"messages": [{"role": "user", "content": "Hello"}]}'
```

`--json` also accepts **JSONC**: `//` and `/* */` comments, trailing commas, and a leading UTF-8 BOM are stripped before parsing. Stripping is string-literal aware, so a `//` in `"https://host/path"` is kept, and a comment-free strict JSON body is unchanged byte-for-byte.

With `--json` and no flags, the body is sent byte-for-byte unchanged. When you also pass flags, only the flags you explicitly set override the matching JSON key; every key you did not touch (including unmodeled vendor keys) is preserved exactly. Flag-to-key mapping follows the normal flag path (`--max-output` → `max_tokens`, `--no-stream` → `stream`, `--temperature` → `temperature`).

```bash
# JSON supplies the messages; the model and token cap come from flags
aigc-cli chat --json '{"messages": [{"role": "user", "content": "Hello"}]}' --model deepseek-v4-flash --max-output 2048
```

Here `messages` stays as written while `model` and `max_tokens` come from the flags. Behavioral flags (`--dry-run`, `--provider`, `--api-key`, `--api-base`, `--output`, `--verbose`, `--timeout`, `--context-size`, `--interactive`, `--search-provider`) never enter the body.

When using `--json` with an `openai_responses` provider, the JSON body must be in Responses shape (`input` / `instructions`), not chat-completions shape (`messages`).

## Injecting an Extra Body Snippet (`--inject-json`)

`--inject-json` sits at the same level as `--json` but is a second body-input channel: it **merges** a small JSON object on top of the body the CLI generated, so vendor parameters the CLI does not model (native web search, reasoning, routing) reach the API without a code change.

- **Merge semantics**: objects recurse; **arrays append** (the CLI's elements first, then yours); every other value overwrites.
- **Precedence**: typed fields/CLI flags < `--json` < `--inject-json` — the injected snippet always wins.
- Input: a file path or inline JSON (**stdin is not supported** — this avoids competing with other stdin consumers such as `--json` / `--prompt`). The value must be a JSON **object**, or the command errors out.
- **Known limitation**: arrays can only be appended, never cleared or replaced; only objects are accepted.

> 💡 The snippet is **provider-specific** — the CLI does not translate it. `{"enable_search":true}` only works on Bailian's OpenAI-compatible protocol; Anthropic-native uses `{"tools":[{"type":"web_search_20250305","name":"web_search"}]}`; OpenAI Responses needs `{"tools":[{"type":"web_search_preview"}]}`; OpenRouter uses `{"plugins":[{"id":"web"}]}` (or the model-name suffix `:online`). Run with `--dry-run` first to see the exact curl.

**Example: OpenRouter web search.** OpenRouter's `web` plugin works for **any model** (non-native models are backed by Exa). It is a chat-completions body field, so the provider must use `type: openai` (`/chat/completions` — **not** `type: anthropic`):

```yaml
providers:
  openrouter:
    type: openai
    api_key: sk-or-xxx
    base_url: https://openrouter.ai/api/v1
```

```bash
aigc-cli chat -P openrouter --model anthropic/claude-sonnet-5.5 \
  --message "New York weather tomorrow" --no-stream \
  --inject-json '{"plugins":[{"id":"web","max_results":5}]}'
```

`plugins` also accepts `engine` (`native`/`exa`/`firecrawl`/…), `include_domains` / `exclude_domains`, and `search_prompt`. To skip the parameters, use the `:online` model suffix instead (equivalent to `plugins:[{"id":"web"}]`): `--model anthropic/claude-sonnet-5.5:online`. Web search is billed per search.

## Provider Type

`config.providers.<name>.type` selects the wire protocol for `chat`:

| Type | Protocol |
|---|---|
| `openai` | OpenAI-compatible (default), `POST /v1/chat/completions` |
| `openai_chat_completions` | Explicit alias of `openai` — classic chat completions |
| `openai_responses` | OpenAI Responses API, `POST {base_url}/responses` |
| `anthropic` | Anthropic Messages API |
| `ollama` | Ollama local |

Use `type: openai_responses` when a reasoning model rejects function tools on `/v1/chat/completions` with HTTP 400 "Function tools with reasoning_effort are not supported ... use /v1/responses". This makes the agent/tool loop work because the Responses API supports tools together with reasoning.

Example config:

```yaml
providers:
  openlux:
    type: openai_responses
    api_key: sk-xxx
    base_url: https://api.openlux.ai
defaults:
  chat:
    provider: openlux
    model: gpt-6-luna
```

Other commands (image, video, audio, music) are unaffected.

## Zero Data Retention (`--zdr`)

`--zdr` asks the provider to retain nothing. OpenRouter is the only current
provider with a request-level control, and it applies **only to its
chat-completions endpoints** (`chat`, plus OpenRouter `music`):

```bash
export OPENAI_API_KEY="sk-or-xxx"
export OPENAI_BASE_URL="https://openrouter.ai/api/v1"

aigc-cli chat --zdr --message "Hello"
# request body includes: "provider": {"zdr": true, "data_collection": "deny"}
```

- Precedence: `--zdr` flag > `providers.{name}.zdr` > global `zdr` > off. Use the
  per-provider form when only some of your accounts support ZDR:
  ```yaml
  providers:
    openrouter:
      base_url: "https://openrouter.ai/api/v1"
      zdr: true
  ```
- OpenRouter then routes only to ZDR-compliant upstreams. If none is available
  the request **fails** (HTTP 404 "No endpoints found matching your data
  policy") instead of silently falling back to a non-ZDR provider.
- Other `provider` fields in a `--json` body are preserved; `zdr` and
  `data_collection` are forced on.
- OpenRouter's **Images and Videos** APIs do not accept `zdr` (video is
  ineligible for ZDR by design), so `--zdr` is a silent no-op for `image` and
  `video` — it never injects an unsupported field.
- Other providers have no request-level ZDR switch (retention is set on the
  account/console side), so `--zdr` is a no-op there too.

## Verbose Output

```bash
aigc-cli chat --message "Hello" --verbose
```

Shows token usage, timing, and cost information.
