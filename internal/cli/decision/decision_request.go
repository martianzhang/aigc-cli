package decision

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/service"
)

// maxQuestions is the API limit on questions per request.
const maxQuestions = 64

// bank is the parsed question bank (JSON/JSONC).
type bank struct {
	Model     string                     `json:"model"`
	State     json.RawMessage            `json:"state"`
	Images    []string                   `json:"images"`
	Questions map[string]json.RawMessage `json:"questions"`
}

// loadBank reads and parses the question bank: --json (file / inline / stdin)
// wins, then defaults.decision.bank from config.
func loadBank() (*bank, error) {
	src := options.Shared.JSONInput
	if src == "" {
		if cfg := options.DecisionDefaults(); cfg != nil {
			src = cfg.Bank
		}
	}
	if src == "" {
		return nil, fmt.Errorf("no question bank: pass --json <path|inline-json|-> or set defaults.decision.bank in config.yaml")
	}
	data, err := service.ReadJSONInput(src)
	if err != nil {
		return nil, fmt.Errorf("read question bank: %w", err)
	}
	var b bank
	if err := json.Unmarshal(data, &b); err != nil {
		return nil, fmt.Errorf("parse question bank: %w", err)
	}
	if len(b.Questions) == 0 {
		return nil, fmt.Errorf("question bank has no questions: expected a \"questions\" object with 1-%d named questions", maxQuestions)
	}
	return &b, nil
}

// filterQuestions selects the requested questions from the bank. An empty
// filter keeps all questions. Unknown names, an empty selection and more
// than maxQuestions entries are errors.
func filterQuestions(questions map[string]json.RawMessage, filter []string) (map[string]json.RawMessage, error) {
	names := sortedQuestionNames(questions)
	if len(filter) == 0 {
		if len(questions) > maxQuestions {
			return nil, fmt.Errorf("too many questions: %d exceeds the API limit of %d", len(questions), maxQuestions)
		}
		return questions, nil
	}
	selected := make(map[string]json.RawMessage, len(filter))
	for _, name := range filter {
		if name = strings.TrimSpace(name); name == "" {
			continue
		}
		raw, ok := questions[name]
		if !ok {
			return nil, fmt.Errorf("unknown question %q (valid: %s)", name, strings.Join(names, ", "))
		}
		selected[name] = raw
	}
	switch {
	case len(selected) == 0:
		return nil, fmt.Errorf("no questions selected by --questions (valid: %s)", strings.Join(names, ", "))
	case len(selected) > maxQuestions:
		return nil, fmt.Errorf("too many questions: %d exceeds the API limit of %d", len(selected), maxQuestions)
	}
	return selected, nil
}

// sortedQuestionNames returns the bank's question names in sorted order.
func sortedQuestionNames(questions map[string]json.RawMessage) []string {
	names := make([]string, 0, len(questions))
	for name := range questions {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// resolveState builds the request state: --state (text / file / stdin) wins,
// then bank.state. Content that is a valid JSON object or array is passed
// through as structured JSON; anything else is sent as a JSON string.
func resolveState(flagValue string, bankState json.RawMessage) (json.RawMessage, error) {
	if flagValue == "" {
		if len(bankState) > 0 {
			return bankState, nil
		}
		return nil, fmt.Errorf("state is required: pass --state <text|path|-> or set \"state\" in the question bank")
	}
	content, err := service.ReadInput(flagValue)
	if err != nil {
		return nil, fmt.Errorf("read state: %w", err)
	}
	trimmed := strings.TrimSpace(string(content))
	if trimmed == "" {
		return nil, fmt.Errorf("state is required: --state resolved to empty content")
	}
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		var probe json.RawMessage
		if err := json.Unmarshal([]byte(trimmed), &probe); err == nil {
			return json.RawMessage(trimmed), nil
		}
	}
	marshaled, err := json.Marshal(trimmed)
	if err != nil {
		return nil, fmt.Errorf("encode state: %w", err)
	}
	return marshaled, nil
}

// resolveModel applies the documented model priority: CLI -m > bank.model >
// defaults.decision.model / provider model > "tev1".
func resolveModel(cliModel, bankModel, epModel string) string {
	for _, m := range []string{cliModel, bankModel, epModel} {
		if m != "" {
			return m
		}
	}
	return "tev1"
}

// resolveImages normalizes attached images to raw base64: each entry must be a
// local PNG/JPEG/WebP file or a base64 data: URI. CLI --image wins over
// bank.images; blank entries are skipped. When maxEdge > 0 every image is
// downscaled so its longest edge is at most maxEdge pixels before encoding,
// which cuts vision tokens and decision latency.
func resolveImages(cliImages, bankImages []string, maxEdge int) ([]string, error) {
	inputs := cliImages
	if len(inputs) == 0 {
		inputs = bankImages
	}
	if len(inputs) == 0 {
		return nil, nil
	}
	images := make([]string, 0, len(inputs))
	for _, in := range inputs {
		in = strings.TrimSpace(in)
		if in == "" {
			continue
		}
		data, label, err := imageInputBytes(in)
		if err != nil {
			return nil, err
		}
		if maxEdge > 0 {
			resized, ok, resizeErr := service.ResizeImageBytes(data, maxEdge)
			switch {
			case resizeErr != nil:
				fmt.Fprintf(os.Stderr, "Warning: resize %s: %v (sending original)\n", label, resizeErr)
			case ok:
				if options.Shared.Verbose {
					fmt.Fprintf(os.Stderr, "Resized %s to longest edge %dpx\n", label, maxEdge)
				}
				data = resized
			}
		}
		images = append(images, base64.StdEncoding.EncodeToString(data))
	}
	return images, nil
}

// imageInputBytes resolves one --image entry to raw image bytes plus a label
// for diagnostics: a validated local file or a base64 data: URI.
func imageInputBytes(in string) ([]byte, string, error) {
	switch {
	case service.IsFile(in):
		data, err := service.ReadImageFile(in)
		return data, in, err
	case strings.HasPrefix(in, "data:"):
		b64, err := stripDataURIBase64(in)
		if err != nil {
			return nil, "data URI", err
		}
		data, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			return nil, "data URI", fmt.Errorf("decode data URI: %w", err)
		}
		return data, "data URI", nil
	default:
		return nil, in, fmt.Errorf("image %q is not a local PNG/JPEG/WebP file or data URI (the decision API does not accept URLs; download the image first)", in)
	}
}

// stripDataURIBase64 extracts the raw base64 payload from a data: URI.
// The bytes themselves are not re-validated; the provider decodes them.
func stripDataURIBase64(uri string) (string, error) {
	comma := strings.Index(uri, ",")
	if comma < 0 {
		return "", fmt.Errorf("invalid data URI: missing comma")
	}
	if !strings.Contains(uri[:comma], ";base64") {
		return "", fmt.Errorf("data URI must be base64-encoded")
	}
	payload := uri[comma+1:]
	if payload == "" {
		return "", fmt.Errorf("data URI contains no data")
	}
	return payload, nil
}
