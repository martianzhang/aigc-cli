package mcp

import (
	"fmt"
	"strings"
)

// toolDoc is the agent-facing contract for an MCP tool. Every registered tool
// has one so a client can decide when to call it without reading the CLI docs.
type toolDoc struct {
	When     string
	Requires string
	Cost     string
	Returns  string
	Example  string
}

// toolDocs covers every entry in toolRegistry. Missing entries are caught by
// TestToolDocsCoverRegistry so a new tool cannot ship without a contract.
var toolDocs = map[string]toolDoc{
	"generate_image": {
		When:     "the user asks to create or edit an image from text or reference images",
		Requires: "image provider and model; API key unless using a local/Ollama endpoint",
		Cost:     "paid per image; use --dry-run on the CLI for a free preview",
		Returns:  "absolute file paths, model/provider, task id when async, and the image media block",
		Example:  `{"prompt":"a cat under starry sky","size":"1024x1024"}`,
	},
	"generate_video": {
		When:     "the user asks for text-to-video, image-to-video, or a video remix",
		Requires: "video provider and model; API key; ffmpeg only for GIF conversion",
		Cost:     "paid per video; async jobs may take minutes",
		Returns:  "absolute video paths, model/provider, task/job id, and optional last-frame paths",
		Example:  `{"prompt":"a kitten yawning","duration":8}`,
	},
	"generate_music": {
		When:     "the user asks to generate music, a song, or background audio",
		Requires: "music backend and model (APIMart/OpenRouter/Bailian) plus API key",
		Cost:     "paid per track; APIMart is async",
		Returns:  "absolute audio paths, model/provider, and task id for async backends",
		Example:  `{"prompt":"city pop","model":"suno"}`,
	},
	"generate_speech": {
		When:     "the user asks to convert text to speech or narrate a script",
		Requires: "TTS model; cloud provider key or local 'aigc-cli audio init'",
		Cost:     "cloud TTS is paid; local inference is free",
		Returns:  "absolute audio path, model, voice, and format",
		Example:  `{"input":"Hello world","voice":"alloy","format":"mp3"}`,
	},
	"generate_sfx": {
		When:     "the user asks for a short sound effect",
		Requires: "SFX model; cloud provider key or local SFX model installed",
		Cost:     "cloud SFX may be paid; local inference is free",
		Returns:  "absolute audio path, model, duration, and seed when available",
		Example:  `{"prompt":"door slam","duration_seconds":1.5}`,
	},
	"transcribe_audio": {
		When:     "the user asks to transcribe a local audio file or piped audio",
		Requires: "STT model; cloud key or local 'aigc-cli audio init'",
		Cost:     "cloud STT is paid; local inference is free",
		Returns:  "transcript text and the saved file path",
		Example:  `{"file_path":"/tmp/talk.mp3","language":"zh"}`,
	},
	"midjourney_imagine": {
		When:     "the user explicitly asks for Midjourney or highly stylized variants",
		Requires: "Midjourney-capable provider and API key",
		Cost:     "paid; typically 5-10x generate_image and returns 4 variants",
		Returns:  "task id and variant/result URLs or saved paths",
		Example:  `{"prompt":"a cute cat --ar 16:9"}`,
	},
	"midjourney_describe": {
		When:     "the user wants a Midjourney-style reverse prompt from an image",
		Requires: "image URL or local file plus a Midjourney-capable provider",
		Cost:     "paid per request",
		Returns:  "four candidate prompts",
		Example:  `{"image_url":"https://example.com/cat.png"}`,
	},
	"midjourney_reroll": {
		When:     "the user wants new results for an existing Midjourney task",
		Requires: "a previous Midjourney task id and provider key",
		Cost:     "paid per reroll",
		Returns:  "new task id and result URLs",
		Example:  `{"task_id":"mj_task_123"}`,
	},
	"midjourney_video": {
		When:     "the user wants to animate an image through Midjourney",
		Requires: "image URL and Midjourney-capable provider",
		Cost:     "paid per video",
		Returns:  "task id and video URL/path",
		Example:  `{"image_url":"https://example.com/cat.png"}`,
	},
	"list_models": {
		When:     "the agent needs a valid model id before generating",
		Requires: "a configured provider (or --api-base); no model required",
		Cost:     "free (metadata query)",
		Returns:  "model ids, names, and type metadata",
		Example:  `{"type":"image","provider":"openrouter"}`,
	},
	"get_model_pricing": {
		When:     "the agent needs to estimate or compare model cost",
		Requires: "a provider that publishes pricing; no model required",
		Cost:     "free (metadata query)",
		Returns:  "pricing rows for the requested model or catalog",
		Example:  `{"model":"gpt-image-2"}`,
	},
	"get_balance": {
		When:     "the agent needs the remaining account balance",
		Requires: "a billing/management-capable provider key",
		Cost:     "free (account query)",
		Returns:  "balance and currency per configured provider",
		Example:  `{"provider":"apimart"}`,
	},
	"get_task": {
		When:     "an async generation returned a task/job id and the agent needs its status or result",
		Requires: "task id or job id; the provider that submitted it",
		Cost:     "free (status query)",
		Returns:  "status, progress, error, and result paths when completed",
		Example:  `{"task_id":"task_123","provider":"apimart"}`,
	},
	"get_config": {
		When:     "the agent needs the effective provider/model/output configuration",
		Requires: "none",
		Cost:     "free",
		Returns:  "provider, model, output directory, and masked settings",
		Example:  `{}`,
	},
	"caption_image": {
		When:     "the user wants to read or write an image caption",
		Requires: "a local image path",
		Cost:     "free",
		Returns:  "current caption or confirmation that it was written",
		Example:  `{"file_path":"/tmp/a.png","caption":"a cat"}`,
	},
	"search_ideas": {
		When:     "the user needs prompt inspiration or a reusable prompt id",
		Requires: "local ideas dataset ('aigc-cli ideas init')",
		Cost:     "free (local BM25)",
		Returns:  "matching prompts with ids and scores",
		Example:  `{"query":"cinematic portrait","limit":5}`,
	},
	"remove_background": {
		When:     "the user wants the subject cut out of an image",
		Requires: "local RMBG model ('aigc-cli background init')",
		Cost:     "free (local ONNX)",
		Returns:  "absolute output path",
		Example:  `{"file_path":"/tmp/cat.png"}`,
	},
	"convert_depth": {
		When:     "the user wants a depth map for an image or video",
		Requires: "local depth model ('aigc-cli depth init'); ffmpeg for video",
		Cost:     "free (local ONNX)",
		Returns:  "absolute depth image/video path",
		Example:  `{"file_path":"/tmp/photo.jpg"}`,
	},
	"upscale": {
		When:     "the user wants a 2x/4x super-resolution upscale",
		Requires: "local upscaler model ('aigc-cli upscale init')",
		Cost:     "free (local ONNX)",
		Returns:  "absolute upscaled image path and dimensions",
		Example:  `{"file_path":"/tmp/a.png","scale":2}`,
	},
	"detect_image": {
		When:     "the user asks whether an image is AI-generated or watermarked",
		Requires: "optional local detect model ('aigc-cli detect init'); metadata works without it",
		Cost:     "free (offline)",
		Returns:  "signal breakdown and an overall verdict",
		Example:  `{"file_path":"/tmp/a.png"}`,
	},
	"remove_watermark": {
		When:     "the user wants a learned or visible watermark removed",
		Requires: "a learned watermark or watermark box",
		Cost:     "free (local)",
		Returns:  "absolute output path",
		Example:  `{"file_path":"/tmp/a.png","watermark_box":"10,10,120,40"}`,
	},
	"add_watermark": {
		When:     "the user wants to add a visible test watermark",
		Requires: "none beyond a local image",
		Cost:     "free (local)",
		Returns:  "absolute output path",
		Example:  `{"file_path":"/tmp/a.png","text":"test"}`,
	},
	"crop_watermark": {
		When:     "a visible watermark can be removed by cropping",
		Requires: "a local image and crop margins",
		Cost:     "free (local)",
		Returns:  "absolute output path",
		Example:  `{"file_path":"/tmp/a.png","margin":"0,0,40,0"}`,
	},
	"recognize_text": {
		When:     "the user wants OCR text from an image or PDF",
		Requires: "local OCR model ('aigc-cli ocr init') or an online parser provider",
		Cost:     "offline is free; online parsing may be paid",
		Returns:  "recognized text and page/line structure",
		Example:  `{"file_path":"/tmp/scan.png","engine":"auto"}`,
	},
	"redact_text": {
		When:     "the user wants PII removed from a text string",
		Requires: "none for built-in rules; optional local model",
		Cost:     "free for rules; optional model/provider calls may be paid",
		Returns:  "redacted text and detected span types",
		Example:  `{"text":"call 13812345678"}`,
	},
	"redact_check": {
		When:     "the user wants to verify a redacted file has no leftovers",
		Requires: "redacted file and mapping file",
		Cost:     "free",
		Returns:  "residual findings and a non-zero status when leftovers exist",
		Example:  `{"file_path":"/tmp/a.redacted.docx","mapping_path":"/tmp/a.map.json"}`,
	},
	"redact_scan": {
		When:     "the user wants PII spans without modifying the file",
		Requires: "text/DOCX/PDF/image file",
		Cost:     "free for rules; optional model may be paid",
		Returns:  "spans with offsets, types, and confidence",
		Example:  `{"file_path":"/tmp/complaint.docx"}`,
	},
	"redact_apply": {
		When:     "the user wants to redact a file and write the result",
		Requires: "input file; mapping path for reversible DOCX",
		Cost:     "free for rules; optional model may be paid",
		Returns:  "output path and mapping path",
		Example:  `{"file_path":"/tmp/complaint.docx","mapping_path":"/tmp/complaint.map.json"}`,
	},
	"redact_restore": {
		When:     "the user wants to restore a redacted DOCX from its mapping",
		Requires: "redacted DOCX and the original mapping file",
		Cost:     "free",
		Returns:  "restored file path",
		Example:  `{"file_path":"/tmp/a.redacted.docx","mapping_path":"/tmp/a.map.json"}`,
	},
	"search": {
		When:     "the user wants a free web search",
		Requires: "network access",
		Cost:     "free (DuckDuckGo built-in)",
		Returns:  "search result JSON",
		Example:  `{"query":"golang context timeout","limit":5}`,
	},
	"fetch": {
		When:     "the user wants a URL converted to Markdown or saved as media",
		Requires: "network access; local file paths also work",
		Cost:     "free (built-in fetcher)",
		Returns:  "Markdown text or the saved file path",
		Example:  `{"url":"https://example.com/article"}`,
	},
	"web_search": {
		When:     "the user wants results from the configured third-party search vendor",
		Requires: "defaults.search.provider and its API key",
		Cost:     "may bill per request",
		Returns:  "vendor JSON",
		Example:  `{"query":"vector database","limit":5}`,
	},
	"web_fetch": {
		When:     "the user wants a page fetched through the third-party fetch API",
		Requires: "defaults.search.provider and its API key",
		Cost:     "may bill per request",
		Returns:  "Markdown or vendor JSON",
		Example:  `{"url":"https://example.com"}`,
	},
	"kb_find": {
		When:     "the user wants to search the local knowledge base",
		Requires: "initialized knowledge base ('aigc-cli kb init')",
		Cost:     "free (local FTS5/embedding)",
		Returns:  "matching documents and chunks",
		Example:  `{"query":"installation proxy"}`,
	},
	"kb_search": {
		When:     "the user wants a web search saved into the knowledge base",
		Requires: "initialized KB and network access",
		Cost:     "search vendor may bill",
		Returns:  "saved document ids",
		Example:  `{"query":"golang generics"}`,
	},
	"kb_add": {
		When:     "the user wants to add a local file to the knowledge base",
		Requires: "initialized KB and a readable path",
		Cost:     "free (embedding may call a provider)",
		Returns:  "document id and chunk count",
		Example:  `{"file_path":"/tmp/notes.md"}`,
	},
	"kb_fetch": {
		When:     "the user wants a URL fetched and saved into the knowledge base",
		Requires: "initialized KB and network access",
		Cost:     "free fetch; embedding may call a provider",
		Returns:  "document id and chunk count",
		Example:  `{"url":"https://example.com/post"}`,
	},
	"kb_list": {
		When:     "the user wants to browse knowledge base documents",
		Requires: "initialized KB",
		Cost:     "free",
		Returns:  "document ids, titles, and URLs",
		Example:  `{"limit":20}`,
	},
	"kb_show": {
		When:     "the user wants the full content of a knowledge base document",
		Requires: "initialized KB and document id",
		Cost:     "free",
		Returns:  "document content and metadata",
		Example:  `{"id":"doc_123"}`,
	},
}

// describeTool appends the contract fields to a tool's base description.
func describeTool(name, base string) string {
	doc, ok := toolDocs[name]
	if !ok {
		return base
	}
	var b strings.Builder
	b.WriteString(base)
	write := func(label, value string) {
		if value == "" {
			return
		}
		fmt.Fprintf(&b, "\n\n%s %s", label, value)
	}
	write("When to use:", doc.When)
	write("Requires:", doc.Requires)
	write("Cost:", doc.Cost)
	write("Returns:", doc.Returns)
	write("Example:", doc.Example)
	return b.String()
}
