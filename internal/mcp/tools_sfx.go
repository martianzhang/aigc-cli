package mcp

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/martianzhang/aigc-cli/internal/audio"
	sfxengine "github.com/martianzhang/aigc-cli/internal/audio/sfx"
	"github.com/martianzhang/aigc-cli/internal/fsutil"
	"github.com/martianzhang/aigc-cli/internal/onnxrt"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// newGenerateSFXTool defines the generate_sfx tool.
func newGenerateSFXTool(desc string) mcp.Tool {
	return mcp.NewTool("generate_sfx",
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(desc),
		mcp.WithString("input",
			mcp.Required(),
			mcp.Description("Sound description, e.g. 'heavy metal door slam with echo'"),
		),
		mcp.WithString("model",
			mcp.Description("Model ID. Online defaults: eleven_text_to_sound_v2 / stable-audio-2.5; local default: stable-audio-open-small"),
		),
		mcp.WithString("provider",
			mcp.Description(providerArgDesc+" Use 'local' for offline ONNX inference."),
		),
		mcp.WithNumber("duration",
			mcp.Description("Duration in seconds. 0 = provider default; local 0.1-11.9, ElevenLabs 0.5-30, Stability 1-190"),
		),
		mcp.WithBoolean("loop",
			mcp.Description("Seamless loop (ElevenLabs only)"),
		),
		mcp.WithNumber("influence",
			mcp.Description("Prompt adherence 0-1 (ElevenLabs only)"),
		),
		mcp.WithString("format",
			mcp.Enum("mp3", "wav"),
			mcp.Description("Audio format (local always wav)"),
		),
		mcp.WithNumber("seed",
			mcp.Description("Random seed (local only; same seed reproduces the output)"),
		),
		mcp.WithBoolean("normalize",
			mcp.Description("Peak-normalize local output to -1 dBFS (default true)"),
		),
	)
}

// buildSFXDesc injects current defaults into the tool description.
func buildSFXDesc(d *types.AudioDefaults, baseURL string) string {
	b := new(strings.Builder)
	p := provider.Detect(baseURL)
	fmt.Fprintf(b, "Generate a short sound effect (SFX) via %s.\n\nCurrent config (edit in ~/.config/aigc-cli/config.yaml):\n", p)
	if d != nil {
		fmt.Fprintf(b, "  sfx.model = %s | sfx.format = %s | sfx.provider = %s\n",
			d.SFXModel(), d.SFXFormat(), d.SFXProvider())
	}
	b.WriteString("\nUse provider=local for offline ONNX inference (requires 'aigc-cli audio init --model stable-audio-open-small').\n")
	b.WriteString("Strategy: parameters already have defaults. Only set when the user explicitly specifies a value. Audio is saved to local files — do NOT invent URLs.\n")
	return b.String()
}

// generateSFXHandler creates the handler for generate_sfx.
func generateSFXHandler(cfg *Config) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		input, err := request.RequireString("input")
		if err != nil {
			return mcp.NewToolResultError("input is required"), nil
		}

		p, providerErr := cfg.resolveProviderRef("audio.sfx", request.GetString("provider", ""))
		if providerErr != "" {
			return mcp.NewToolResultError(providerErr), nil
		}

		model := request.GetString("model", "")
		if model == "" && cfg.Defaults != nil && cfg.Defaults.Audio != nil {
			model = cfg.Defaults.Audio.SFXModel()
		}
		duration := request.GetFloat("duration", 0)

		if p.Type == types.ProviderLocal {
			return generateLocalSFX(cfg, request, input, model, duration)
		}
		return generateOnlineSFX(cfg, request, p, input, model, duration)
	}
}

func generateOnlineSFX(cfg *Config, request mcp.CallToolRequest, p *provider.EffectiveProvider, prompt, model string, duration float64) (*mcp.CallToolResult, error) {
	if p.RequiresAPIKey() {
		return mcp.NewToolResultError("API Key not configured"), nil
	}

	format := strings.ToLower(request.GetString("format", ""))
	if format == "" && cfg.Defaults != nil && cfg.Defaults.Audio != nil {
		format = strings.ToLower(cfg.Defaults.Audio.SFXFormat())
	}
	if format == "" {
		format = "mp3"
	}
	if format != "mp3" && format != "wav" {
		return mcp.NewToolResultError("format must be mp3 or wav"), nil
	}

	req := &provider.SoundEffectRequest{
		Prompt:          prompt,
		DurationSeconds: duration,
		Loop:            request.GetBool("loop", false),
		Model:           model,
		Format:          format,
	}
	if args := request.GetArguments(); args != nil {
		if _, ok := args["influence"]; ok {
			v := request.GetFloat("influence", 0.3)
			req.PromptInfluence = &v
		}
	}

	switch p.ProviderType {
	case provider.ElevenLabs:
		if req.DurationSeconds > 0 && (req.DurationSeconds < 0.5 || req.DurationSeconds > 30) {
			return mcp.NewToolResultError("elevenlabs duration must be 0.5-30s"), nil
		}
		if req.Loop && req.DurationSeconds > 22 {
			return mcp.NewToolResultError("elevenlabs looping sounds are limited to 22s"), nil
		}
		if req.Model == "" {
			req.Model = provider.DefaultSFXModelElevenLabs
		}
	case provider.Stability:
		if req.Loop {
			return mcp.NewToolResultError("stability does not support loop"), nil
		}
		if req.PromptInfluence != nil {
			return mcp.NewToolResultError("stability does not support influence"), nil
		}
		if req.DurationSeconds == 0 {
			req.DurationSeconds = 5
		}
		if req.DurationSeconds < 1 || req.DurationSeconds > 190 {
			return mcp.NewToolResultError("stability duration must be 1-190s"), nil
		}
		if req.Model == "" {
			req.Model = provider.DefaultSFXModelStability
		}
	default:
		return mcp.NewToolResultError(fmt.Sprintf("provider %q does not support sound effects; use provider=local or configure ElevenLabs/Stability", p.Name)), nil
	}

	res, err := provider.GenerateSoundEffect(p, req)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("sound effect failed: %v", err)), nil
	}

	filename := filepath.Join(cfg.Output, fmt.Sprintf("sfx_%d.%s", time.Now().Unix(), format))
	if err := fsutil.WriteFileAtomic(filename, res.Data, 0644); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to save audio: %v", err)), nil
	}
	return toolResultTextWithMedia(fmt.Sprintf(
		"Sound effect saved: %s\nFormat: %s\nSize: %d bytes\nModel: %s",
		filename, format, len(res.Data), req.Model), filename), nil
}

func generateLocalSFX(cfg *Config, request mcp.CallToolRequest, prompt, model string, duration float64) (*mcp.CallToolResult, error) {
	if model == "" {
		model = audio.SFXDefaultModelID
	}
	if cfg.ModelsDir == "" {
		return mcp.NewToolResultError("local models directory is not configured"), nil
	}

	modelDir := filepath.Join(cfg.ModelsDir, "audio", model)
	if filepath.IsAbs(model) {
		modelDir = model
	}
	libPath, err := onnxrt.LibPath(cfg.ModelsDir)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("%v; run 'aigc-cli audio init --model %s'", err, audio.SFXDefaultModelID)), nil
	}
	if duration <= 0 {
		duration = 5
	}

	engine, err := sfxengine.NewEngine(modelDir, libPath)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("load sfx model: %v (run 'aigc-cli audio init --model %s')", err, audio.SFXDefaultModelID)), nil
	}
	defer engine.Close()

	generated, err := engine.Generate(sfxengine.Request{
		Prompt:    prompt,
		Seconds:   duration,
		Seed:      int64(request.GetInt("seed", 0)),
		Normalize: request.GetBool("normalize", true),
	})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("sound effect failed: %v", err)), nil
	}

	wav, err := sfxengine.EncodeWAV(generated.Samples, generated.Channels, generated.SampleRate)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("encode wav: %v", err)), nil
	}
	filename := filepath.Join(cfg.Output, fmt.Sprintf("sfx_%d.wav", time.Now().Unix()))
	if err := fsutil.WriteFileAtomic(filename, wav, 0644); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to save audio: %v", err)), nil
	}
	return toolResultTextWithMedia(fmt.Sprintf(
		"Sound effect saved: %s\nFormat: wav %dHz %dch\nDuration: %.2fs\nSeed: %d",
		filename, generated.SampleRate, generated.Channels, generated.Duration(), generated.Seed), filename), nil
}
