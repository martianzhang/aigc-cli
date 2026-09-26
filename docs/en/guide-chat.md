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
- Tool calling (image generation, video, Midjourney, ideas, knowledge base)
- Streaming output
- Command history

## Tool Calling

In REPL mode, the AI can use built-in tools:

| Tool | Description |
|---|---|
| `generate_image` | Generate images |
| `generate_video` | Generate videos |
| `generate_music` | Generate music |
| `midjourney_imagine` | Midjourney text-to-image |
| `search_ideas` | Search prompt ideas |
| `kb_find` | Search knowledge base |
| `kb_search` | Web search + save to KB |
| `detect_image` | AIGC detection |
| `remove_watermark` | Watermark removal |
| `convert_depth` | Image/video → grayscale depth map (offline, V2 models) |

Tools can be enabled/disabled via config:

```yaml
# Only allow free/offline tools
tools_enable:
  - "search_ideas"
  - "detect_image"
  - "recognize_text"

# Block paid generation tools
tools_disable:
  - "generate_image"
  - "generate_video"
```

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
| `--dry-run` | Print the equivalent curl (real endpoint + body) without calling the API | — |

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

Here `messages` stays as written while `model` and `max_tokens` come from the flags. Behavioral flags (`--dry-run`, `--provider`, `--api-key`, `--api-base`, `--output`, `--verbose`, `--timeout`, `--context-size`, `--interactive`) never enter the body.

When using `--json` with an `openai_responses` provider, the JSON body must be in Responses shape (`input` / `instructions`), not chat-completions shape (`messages`).

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
