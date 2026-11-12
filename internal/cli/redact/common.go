package redact

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/docredact"
	"github.com/martianzhang/aigc-cli/internal/fsutil"
	"github.com/martianzhang/aigc-cli/internal/options"
	"github.com/martianzhang/aigc-cli/internal/provider"
	redactlib "github.com/martianzhang/aigc-cli/internal/redact"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// Deps carries runtime configuration for the redact command.
type Deps struct {
	OutputDir       string
	Verbose         bool
	Cfg             *types.Config
	ResolveProvider func(string) *provider.EffectiveProvider
}

var d Deps

var depsFn func() Deps

// NewCommand builds the `redact` command tree.
func NewCommand(deps func() Deps) *cobra.Command {
	depsFn = deps
	cmd := &cobra.Command{
		Use:          "redact",
		Short:        "Detect and redact PII in text, Word, PDF and images",
		SilenceUsage: true,
		Long: `Detect and redact sensitive data in plain text, DOCX, PDF and images.

Plain text (txt/md/csv/json/stdin) uses rules and an optional model. DOCX
preserves formatting and supports mapping/restore. PDF and images are rendered
or OCR'd and redacted with black boxes; their output is intentionally
irreversible.

The default engine is pure Go rules + checksum validators: no model, no network,
and no download. It covers ID cards, phones, bank cards, email, unified social
credit codes, case numbers, plates, passports and lawyer licence numbers.

An optional model engine can be layered on top through any OpenAI-compatible
provider (Ollama, llama-server, vLLM, LM Studio) or a local ONNX
token-classification model. It is used for recall and open-set types such as
organizations, aliases and addresses. Values returned by a model are located in
the text and, by default, only replaced when a role word such as
原告/联系人/电话 appears nearby; ambiguous occurrences are reported by 'scan'
instead of being applied.

Examples:
  # Detect, review, redact and verify
  aigc-cli redact scan contract.txt
  aigc-cli redact scan contract.txt --json > spans.json
  aigc-cli redact apply contract.txt --mapping contract.map.json
  aigc-cli redact check contract.redacted.txt --mapping contract.map.json
  aigc-cli redact restore contract.redacted.txt --mapping contract.map.json

  # Optional model layer: Ollama/HaS or a local ONNX classifier
  aigc-cli --provider ollama --model has4 redact scan contract.txt --engine auto
  aigc-cli --provider local --model-engine tokencls redact scan contract.txt --engine auto

  # Inspect rules/styles and switch profiles
  aigc-cli redact rules
  aigc-cli redact scan contract.txt --profile strict

  # Word / PDF / image: format is detected automatically
  aigc-cli redact apply contract.docx --mapping contract.map.json
  aigc-cli redact apply judgment.pdf --dpi 300
  aigc-cli redact apply scan.png`,
	}
	cmd.AddCommand(newScanCmd(), newApplyCmd(), newRestoreCmd(), newCheckCmd(), newRulesCmd(), newInitCmd())
	return cmd
}

func loadDeps() {
	if depsFn != nil {
		d = depsFn()
	}
}

// buildFlags are the detection flags shared by scan/apply/check.
type buildFlags struct {
	engine        string
	style         string
	enable        string
	disable       string
	rulesFile     string
	dpi           int
	profile       string
	configFile    string
	modelEngine   string
	tokenclsModel string
	allowRemote   bool
}

func addRuleFlags(cmd *cobra.Command, f *buildFlags) {
	cmd.Flags().StringVar(&f.enable, "enable", "", "Comma-separated rule names to enable (in addition to the configured set)")
	cmd.Flags().StringVar(&f.disable, "disable", "", "Comma-separated rule names to disable")
	cmd.Flags().StringVar(&f.rulesFile, "rules-file", "", "YAML file overriding defaults.redact rules / overrides / custom")
	cmd.Flags().StringVar(&f.profile, "profile", "", "Redact profile name from defaults.redact.profiles")
	cmd.Flags().StringVar(&f.configFile, "redact-config", "", "External redact YAML overlaid on defaults.redact")
	cmd.Flags().StringVar(&f.modelEngine, "model-engine", "", "Model backend: auto, chatner, tokencls; the model name goes in --model")
	cmd.Flags().StringVar(&f.tokenclsModel, "tokencls-model", "", "Override the tokencls model pack for this call (e.g. zh-pii, multilingual-ner)")
}

func addDetectFlags(cmd *cobra.Command, f *buildFlags) {
	addRuleFlags(cmd, f)
	cmd.Flags().StringVar(&f.engine, "engine", "", "Engine: rules (default), model, auto")
	cmd.Flags().IntVar(&f.dpi, "dpi", 300, "PDF render DPI")
	cmd.Flags().StringVar(&f.style, "style", "", "Replacement style: placeholder, mask, black, hash")
	cmd.Flags().BoolVar(&f.allowRemote, "allow-remote", false, "Allow a non-loopback model endpoint (dangerous: original text leaves this machine)")
}

func (f buildFlags) redactConfig() (*types.RedactConfig, error) {
	cfg := &types.RedactConfig{}
	if src := options.RedactConfig(); src != nil {
		*cfg = *src
		if src.Rules != nil {
			cfg.Rules = make(map[string]bool, len(src.Rules))
			for k, v := range src.Rules {
				cfg.Rules[k] = v
			}
		}
		cfg.Custom = append([]types.RedactRule(nil), src.Custom...)
	}
	if err := applyRedactFile(cfg, f); err != nil {
		return nil, err
	}
	if f.rulesFile != "" {
		if err := mergeRulesFile(cfg, f.rulesFile); err != nil {
			return nil, err
		}
	}
	if f.engine != "" {
		cfg.Engine = f.engine
	}
	if f.style != "" {
		cfg.Style = f.style
	}
	if f.modelEngine != "" {
		mode := strings.ToLower(strings.TrimSpace(f.modelEngine))
		switch mode {
		case "auto", "chatner", "tokencls":
			cfg.ModelEngine = mode
		default:
			return nil, fmt.Errorf("unknown model engine %q (want auto, chatner or tokencls; use --model %s to select the model name)", f.modelEngine, f.modelEngine)
		}
	}
	if f.enable != "" {
		if cfg.Rules == nil {
			cfg.Rules = map[string]bool{}
		}
		for _, name := range splitCSV(f.enable) {
			names := append([]string{name}, redactlib.RuleFamily(name)...)
			for _, enabled := range names {
				custom := false
				for i := range cfg.Custom {
					if cfg.Custom[i].Name == enabled {
						on := true
						cfg.Custom[i].Enabled = &on
						custom = true
					}
				}
				if !custom {
					cfg.Rules[enabled] = true
				}
			}
		}
	}
	for _, name := range splitCSV(f.disable) {
		if cfg.Rules == nil {
			cfg.Rules = map[string]bool{}
		}
		names := append([]string{name}, redactlib.RuleFamily(name)...)
		for _, disabled := range names {
			custom := false
			for i := range cfg.Custom {
				if cfg.Custom[i].Name == disabled {
					off := false
					cfg.Custom[i].Enabled = &off
					custom = true
				}
			}
			if !custom {
				cfg.Rules[disabled] = false
			}
		}
	}
	return cfg, nil
}

// applyRedactFile overlays an external redact config. CLI --redact-config wins,
// then --profile, then defaults.redact.profile, then defaults.redact.config_file.
// Paths from config.yaml are relative to that file; CLI paths are cwd-relative.
func applyRedactFile(cfg *types.RedactConfig, f buildFlags) error {
	baseDir := dictionaryBaseDir()
	path := ""
	switch {
	case f.configFile != "":
		path = f.configFile
	case f.profile != "":
		resolved, err := resolveRedactProfile(cfg, f.profile)
		if err != nil {
			return err
		}
		path = resolveConfigRelative(baseDir, resolved)
	case cfg.Profile != "":
		resolved, err := resolveRedactProfile(cfg, cfg.Profile)
		if err != nil {
			return err
		}
		path = resolveConfigRelative(baseDir, resolved)
	case cfg.ConfigFile != "":
		path = resolveConfigRelative(baseDir, cfg.ConfigFile)
	}
	if path == "" {
		return nil
	}
	return mergeRedactFile(cfg, path)
}

func resolveRedactProfile(cfg *types.RedactConfig, name string) (string, error) {
	if path := strings.TrimSpace(cfg.Profiles[name]); path != "" {
		return path, nil
	}
	names := make([]string, 0, len(cfg.Profiles))
	for key := range cfg.Profiles {
		names = append(names, key)
	}
	sort.Strings(names)
	if len(names) == 0 {
		return "", fmt.Errorf("redact profile %q not found (defaults.redact.profiles is empty)", name)
	}
	return "", fmt.Errorf("redact profile %q not found (available: %s)", name, strings.Join(names, ", "))
}

func resolveConfigRelative(baseDir, path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(baseDir, path)
}

// mergeRedactFile deep-merges a standalone redact YAML over cfg. The file may
// be a bare redact section, or wrapped in redact:/defaults.redact:. Maps such
// as rules are merged key-by-key; scalars and lists replace the base value.
func mergeRedactFile(cfg *types.RedactConfig, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read redact config %s: %w", path, err)
	}
	var overlay map[string]any
	if err := yaml.Unmarshal(data, &overlay); err != nil {
		return fmt.Errorf("parse redact config %s: %w", path, err)
	}
	overlay = unwrapRedactOverlay(overlay)

	baseData, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("encode current redact config: %w", err)
	}
	base := map[string]any{}
	if err := yaml.Unmarshal(baseData, &base); err != nil {
		return fmt.Errorf("decode current redact config: %w", err)
	}
	mergedData, err := yaml.Marshal(deepMergeMap(base, overlay))
	if err != nil {
		return fmt.Errorf("merge redact config %s: %w", path, err)
	}
	var merged types.RedactConfig
	if err := yaml.Unmarshal(mergedData, &merged); err != nil {
		return fmt.Errorf("load redact config %s: %w", path, err)
	}
	*cfg = merged
	return nil
}

func unwrapRedactOverlay(raw map[string]any) map[string]any {
	if section, ok := raw["redact"].(map[string]any); ok {
		return section
	}
	if defaults, ok := raw["defaults"].(map[string]any); ok {
		if section, ok := defaults["redact"].(map[string]any); ok {
			return section
		}
	}
	return raw
}

func deepMergeMap(base, overlay map[string]any) map[string]any {
	if base == nil {
		base = map[string]any{}
	}
	for key, value := range overlay {
		overlayMap, isMap := value.(map[string]any)
		baseMap, baseIsMap := base[key].(map[string]any)
		if isMap && baseIsMap {
			base[key] = deepMergeMap(baseMap, overlayMap)
			continue
		}
		base[key] = value
	}
	return base
}

func mergeRulesFile(cfg *types.RedactConfig, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read rules file: %w", err)
	}
	var file types.RedactConfig
	if err := yaml.Unmarshal(data, &file); err != nil {
		return fmt.Errorf("parse rules file: %w", err)
	}
	if file.Rules != nil {
		cfg.Rules = file.Rules
	}
	if file.Overrides != nil {
		cfg.Overrides = file.Overrides
	}
	if len(file.Custom) > 0 {
		cfg.Custom = file.Custom
	}
	return nil
}

func buildPipeline(f buildFlags) (*redactlib.Pipeline, *redactlib.MaskResolver, error) {
	cfg, err := f.redactConfig()
	if err != nil {
		return nil, nil, err
	}
	if err := validateRuleFlags(cfg, f); err != nil {
		return nil, nil, err
	}
	if cfg.Placeholder != "" &&
		(!strings.Contains(cfg.Placeholder, "{type}") || !strings.Contains(cfg.Placeholder, "{n}")) {
		return nil, nil, fmt.Errorf("redact.placeholder %q must contain {type} and {n}", cfg.Placeholder)
	}
	rules, err := redactlib.NewRuleEngine(cfg)
	if err != nil {
		return nil, nil, err
	}
	masks, err := redactlib.NewMaskResolver(cfg)
	if err != nil {
		return nil, nil, err
	}
	engine := cfg.Engine
	if engine == "" {
		engine = "rules"
	}
	if engine != "rules" && engine != "model" && engine != "auto" {
		return nil, nil, fmt.Errorf("unknown engine %q (want rules, model or auto)", engine)
	}
	var ruleLayer *redactlib.RuleEngine
	if engine != "model" {
		ruleLayer = rules
	}

	var engines []redactlib.Engine
	dict, err := redactlib.NewDictionaryEngine(cfg, dictionaryBaseDir())
	if err != nil {
		return nil, nil, err
	}
	if dict != nil {
		engines = append(engines, dict)
	}
	if engine == "model" || engine == "auto" {
		p := d.ResolveProvider(options.ProviderNameRedact)
		tcCfg := cfg.Tokencls
		mode := "auto"
		if cfg.ModelEngine != "" {
			mode = strings.ToLower(cfg.ModelEngine)
		}
		var useTokencls bool
		switch mode {
		case "tokencls":
			useTokencls = true
		case "chatner":
			useTokencls = false
		case "auto":
			switch {
			case p != nil && p.Type == types.ProviderLocal:
				useTokencls = true
			case p != nil && p.BaseURL != "":
				// An explicit non-local provider (Ollama, vLLM, ...) means the
				// chat/value-level backend, even when tokencls is configured.
				useTokencls = false
			default:
				useTokencls = tcCfg != nil
			}
		default:
			return nil, nil, fmt.Errorf("unknown model engine %q (want auto, chatner or tokencls)", mode)
		}
		if useTokencls {
			if tcCfg == nil {
				tcCfg = &types.RedactTokenclsConfig{}
			}
			if f.tokenclsModel != "" {
				copy := *tcCfg
				copy.Model = f.tokenclsModel
				tcCfg = &copy
			}
			modelsDir := ""
			if p != nil {
				modelsDir = p.ModelsDir
				if tcCfg.Model == "" {
					tcCfg.Model = p.Model
				}
			}
			if modelsDir == "" {
				modelsDir = filepath.Join(options.ConfigDir(), "models")
			}
			eng, err := redactlib.NewTokenClassifier(tcCfg, modelsDir)
			if err != nil {
				if engine == "model" {
					return nil, nil, err
				}
				fmt.Fprintf(os.Stderr, "warning: redact ONNX model disabled: %v\n", err)
			} else {
				engines = append(engines, eng)
			}
		} else if p == nil || p.BaseURL == "" {
			if engine == "model" {
				return nil, nil, fmt.Errorf("engine=model requires a provider; set defaults.redact.provider or pass --provider")
			}
		} else {
			if !f.allowRemote && !provider.IsLocalEndpoint(p.BaseURL) {
				return nil, nil, fmt.Errorf("model endpoint %s is not loopback; pass --allow-remote only if sending original text is acceptable", p.BaseURL)
			}
			model := p.Model
			if model == "" {
				if rd := options.RedactDefaults(); rd != nil {
					model = rd.Model
				}
			}
			if model == "" {
				model = "has4"
			}
			eng, err := redactlib.NewChatner(client.NewFromProvider(p), model, cfg.Chatner)
			if err != nil {
				return nil, nil, err
			}
			engines = append(engines, eng)
		}
	}
	return redactlib.NewPipeline(ruleLayer, engines...), masks, nil
}

func dictionaryBaseDir() string {
	if options.Shared.CfgFile != "" {
		if abs, err := filepath.Abs(options.Shared.CfgFile); err == nil {
			return filepath.Dir(abs)
		}
		return filepath.Dir(options.Shared.CfgFile)
	}
	return options.ConfigDir()
}

func validateRuleFlags(cfg *types.RedactConfig, f buildFlags) error {
	known := map[string]bool{}
	for _, name := range redactlib.BuiltinRuleNames() {
		known[name] = true
	}
	for _, r := range cfg.Custom {
		known[r.Name] = true
	}
	for _, name := range append(splitCSV(f.enable), splitCSV(f.disable)...) {
		if !known[name] {
			return fmt.Errorf("unknown rule %q in --enable/--disable", name)
		}
	}
	return nil
}

func splitCSV(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

const maxInputBytes = 20 << 20 // 20 MB, matching the design's v1 limit

func readInput(path string) (id, text string, err error) {
	if path == "-" {
		b, err := io.ReadAll(io.LimitReader(os.Stdin, maxInputBytes+1))
		if err != nil {
			return "", "", fmt.Errorf("read stdin: %w", err)
		}
		if len(b) > maxInputBytes {
			return "", "", fmt.Errorf("input exceeds %d MB limit", maxInputBytes>>20)
		}
		return "stdin", string(b), nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", "", fmt.Errorf("read %s: %w", path, err)
	}
	if len(b) > maxInputBytes {
		return "", "", fmt.Errorf("input %s exceeds %d MB limit", path, maxInputBytes>>20)
	}
	return filepath.Base(path), string(b), nil
}

func writeOutput(w io.Writer, path, text string) error {
	if path == "-" {
		_, err := fmt.Fprint(w, text)
		return err
	}
	if err := fsutil.WriteFileAtomic(path, []byte(text), fsutil.PrivateFileMode); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// summaryTo prints the stderr summary for text written to target ("-" means
// stdout). When the text went to stdout the summary is separated by a blank
// line, so it never glues onto output that did not end with a newline.
func summaryTo(w io.Writer, target, text, format string, args ...any) {
	if target == "-" {
		if text != "" && !strings.HasSuffix(text, "\n") {
			fmt.Fprintln(w)
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintf(w, format, args...)
}

// prepareRedactInput converts legacy .doc to a temporary .docx. It returns the
// path to operate on, the output extension, and a cleanup function.
func prepareRedactInput(input string) (string, string, func(), error) {
	if input == "-" {
		return input, "", func() {}, nil
	}
	return docredact.PrepareInput(input)
}

func defaultOut(input, suffix string) string {
	if input == "-" {
		return "-"
	}
	ext := filepath.Ext(input)
	stem := strings.TrimSuffix(filepath.Base(input), ext)
	dir := d.OutputDir
	if dir == "" {
		dir = "."
	}
	return filepath.Join(dir, stem+suffix+ext)
}

func defaultRestoreOut(input string) string {
	if input == "-" {
		return "-"
	}
	ext := filepath.Ext(input)
	stem := strings.TrimSuffix(filepath.Base(input), ext)
	stem = strings.TrimSuffix(stem, ".redacted")
	dir := d.OutputDir
	if dir == "" {
		dir = "."
	}
	return filepath.Join(dir, stem+".restored"+ext)
}

func ensureNotInput(input, output string) error {
	if input == "-" || output == "-" {
		return nil
	}
	in, err := filepath.Abs(input)
	if err != nil {
		return err
	}
	out, err := filepath.Abs(output)
	if err != nil {
		return err
	}
	if filepath.Clean(in) == filepath.Clean(out) {
		return fmt.Errorf("refusing to overwrite input %s (choose another -o/--output directory)", input)
	}
	return nil
}
