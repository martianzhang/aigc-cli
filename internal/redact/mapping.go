package redact

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/martianzhang/aigc-cli/internal/fsutil"
	"github.com/martianzhang/aigc-cli/internal/secret"
)

const mappingVersion = 1

// ApplyOptions controls placeholder assignment and replacement.
type ApplyOptions struct {
	Style       string
	Placeholder string
	Date        bool
	Amount      bool
	Aliases     map[string][]string
	Masks       *MaskResolver
}

// Apply replaces spans and returns the output plus a reversible mapping.
func Apply(doc *Document, spans []Span, opts ApplyOptions) (string, *Mapping, error) {
	style := opts.Style
	if style == "" {
		style = "placeholder"
	}
	tpl := opts.Placeholder
	if tpl == "" {
		tpl = "<{type}_{n}>"
	}
	if !strings.Contains(tpl, "{type}") || !strings.Contains(tpl, "{n}") {
		return "", nil, fmt.Errorf("placeholder %q must contain {type} and {n}", tpl)
	}
	aliasOf, aliasesOf, err := buildAliasIndex(opts.Aliases)
	if err != nil {
		return "", nil, err
	}
	if opts.Masks == nil {
		opts.Masks, _ = NewMaskResolver(nil)
	}

	kept := make([]Span, 0, len(spans))
	for _, s := range spans {
		if s.Type == "DATE" && !opts.Date {
			continue
		}
		if s.Type == "AMOUNT" && !opts.Amount {
			continue
		}
		kept = append(kept, s)
	}
	sort.Slice(kept, func(i, j int) bool {
		if kept[i].Start != kept[j].Start {
			return kept[i].Start < kept[j].Start
		}
		return kept[i].End < kept[j].End
	})

	m := &Mapping{
		Version: mappingVersion, CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Doc: doc.ID, Style: style, Placeholder: tpl,
		SourceSHA256: doc.SHA256(),
	}
	entityIdx := map[string]int{}
	counter := map[string]int{}
	var out strings.Builder
	outRunes := 0
	cursor := 0
	mixed := false

	placeholderFor := func(s Span) (string, *Entity, error) {
		primary := s.Text
		if p, ok := aliasOf[s.Text]; ok {
			primary = p
		}
		key := s.Type + "\x00" + primary
		if idx, ok := entityIdx[key]; ok {
			m.Entities[idx].Count++
			return m.Entities[idx].Placeholder, &m.Entities[idx], nil
		}
		counter[s.Type]++
		ph := renderPlaceholder(tpl, s.Type, counter[s.Type])
		if style == "hash" {
			sum := sha256.Sum256([]byte(primary))
			ph = "<" + s.Type + "_" + hex.EncodeToString(sum[:4]) + ">"
		}
		e := Entity{Type: s.Type, Value: primary, Placeholder: ph, Count: 1,
			Aliases: aliasesOf[primary]}
		m.Entities = append(m.Entities, e)
		entityIdx[key] = len(m.Entities) - 1
		return ph, &m.Entities[len(m.Entities)-1], nil
	}

	for _, s := range kept {
		if s.Start < cursor || s.End > len(doc.Text) || s.Start >= s.End {
			continue
		}
		ph, _, err := placeholderFor(s)
		if err != nil {
			return "", nil, err
		}
		styleFor := s.Style
		if styleFor == "" {
			styleFor = style
		}
		if styleFor != "placeholder" {
			mixed = true
		}
		replacement := ph
		masked := ""
		switch styleFor {
		case "placeholder":
		case "mask":
			policy := opts.Masks.Policy(s.Type)
			if s.Mask != nil {
				applyMaskConfig(&policy, s.Mask)
				if err := compileMask(&policy); err != nil {
					return "", nil, fmt.Errorf("mask %s: %w", s.Type, err)
				}
			}
			replacement = MaskValue(s.Text, s.Type, policy)
			masked = replacement
		case "black":
			replacement = strings.Repeat("█", lenRunes(s.Text))
			masked = replacement
		case "hash":
		default:
			return "", nil, fmt.Errorf("unknown style %q", styleFor)
		}

		segment := doc.Text[cursor:s.Start]
		out.WriteString(segment)
		outRunes += lenRunes(segment)
		startRune := outRunes
		out.WriteString(replacement)
		outRunes += lenRunes(replacement)
		m.Occurrences = append(m.Occurrences, Occurrence{
			Doc: doc.ID, Start: startRune, End: outRunes,
			Placeholder: ph, Masked: masked,
		})
		cursor = s.End
	}
	tail := doc.Text[cursor:]
	out.WriteString(tail)
	output := out.String()
	sum := sha256.Sum256([]byte(output))
	m.OutputSHA256 = hex.EncodeToString(sum[:])
	if mixed && style == "placeholder" {
		m.Style = "mixed"
	}
	return output, m, nil
}

func renderPlaceholder(tpl, typ string, n int) string {
	s := strings.ReplaceAll(tpl, "{type}", typ)
	return strings.ReplaceAll(s, "{n}", strconv.Itoa(n))
}

func buildAliasIndex(aliases map[string][]string) (map[string]string, map[string][]string, error) {
	aliasOf := map[string]string{}
	aliasesOf := map[string][]string{}
	for primary, list := range aliases {
		primary = strings.TrimSpace(primary)
		if primary == "" {
			return nil, nil, fmt.Errorf("redact.aliases: empty primary value")
		}
		clean := make([]string, 0, len(list))
		for _, a := range list {
			a = strings.TrimSpace(a)
			if a == "" || a == primary {
				continue
			}
			if prev, ok := aliasOf[a]; ok && prev != primary {
				return nil, nil, fmt.Errorf("redact.aliases: %q belongs to both %q and %q", a, prev, primary)
			}
			aliasOf[a] = primary
			clean = append(clean, a)
		}
		aliasesOf[primary] = clean
	}
	return aliasOf, aliasesOf, nil
}

// SaveMapping writes a mapping file, optionally encrypted with the master key.
func SaveMapping(path string, m *Mapping, encrypt bool) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal mapping: %w", err)
	}
	if encrypt {
		payload, err := secret.EncryptString(string(data))
		if err != nil {
			return fmt.Errorf("encrypt mapping: %w", err)
		}
		env := struct {
			Version   int    `json:"version"`
			Encrypted bool   `json:"encrypted"`
			Payload   string `json:"payload"`
		}{Version: mappingVersion, Encrypted: true, Payload: payload}
		data, err = json.MarshalIndent(env, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal mapping envelope: %w", err)
		}
	}
	if err := fsutil.WriteFileAtomic(path, append(data, '\n'), fsutil.PrivateFileMode); err != nil {
		return fmt.Errorf("write mapping: %w", err)
	}
	return nil
}

// LoadMapping reads a mapping file, decrypting it when needed.
func LoadMapping(path string) (*Mapping, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read mapping: %w", err)
	}
	var env struct {
		Version   int             `json:"version"`
		Encrypted bool            `json:"encrypted"`
		Payload   string          `json:"payload"`
		Mapping   json.RawMessage `json:"mapping"`
	}
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("parse mapping: %w", err)
	}
	if env.Encrypted {
		plain, err := secret.DecryptString(env.Payload)
		if err != nil {
			return nil, fmt.Errorf("decrypt mapping: %w", err)
		}
		data = []byte(plain)
	} else if len(env.Mapping) > 0 {
		data = env.Mapping
	}
	var m Mapping
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse mapping: %w", err)
	}
	if m.Version == 0 {
		m.Version = mappingVersion
	}
	return &m, nil
}

// Restore replaces placeholders with their original values. Only placeholder
// style is reversible; mask/black output is intentionally one-way.
func Restore(text string, m *Mapping, force bool) (string, error) {
	if m == nil {
		return "", fmt.Errorf("mapping is required")
	}
	if m.Style != "" && m.Style != "placeholder" {
		return "", fmt.Errorf("restore requires style=placeholder (mapping style is %q)", m.Style)
	}
	doc := NewDocument(m.Doc, text)
	if m.OutputSHA256 != "" && !force {
		sum := sha256.Sum256([]byte(text))
		if hex.EncodeToString(sum[:]) != m.OutputSHA256 {
			return "", fmt.Errorf("document changed since redaction (sha256 mismatch); use --force to override")
		}
	}
	values := map[string]string{}
	for _, e := range m.Entities {
		values[e.Placeholder] = e.Value
	}
	occ := append([]Occurrence(nil), m.Occurrences...)
	sort.Slice(occ, func(i, j int) bool { return occ[i].Start < occ[j].Start })
	var out strings.Builder
	cursor := 0
	for _, o := range occ {
		value, ok := values[o.Placeholder]
		if !ok {
			return "", fmt.Errorf("mapping has no value for %s", o.Placeholder)
		}
		start := doc.ByteOffset(o.Start)
		end := doc.ByteOffset(o.End)
		if start < cursor || end < start || end > len(text) {
			return "", fmt.Errorf("mapping occurrence out of range (%d..%d)", o.Start, o.End)
		}
		if !force && text[start:end] != o.Placeholder {
			return "", fmt.Errorf("placeholder mismatch at offset %d: found %q, want %q", o.Start, text[start:end], o.Placeholder)
		}
		out.WriteString(text[cursor:start])
		out.WriteString(value)
		cursor = end
	}
	out.WriteString(text[cursor:])
	return out.String(), nil
}

// Check scans text for any original value (or alias) recorded in mapping.
func Check(text string, m *Mapping) []Residual {
	if m == nil {
		return nil
	}
	doc := NewDocument(m.Doc, text)
	var out []Residual
	seen := map[string]bool{}
	for _, e := range m.Entities {
		values := append([]string{e.Value}, e.Aliases...)
		for _, v := range values {
			if v == "" {
				continue
			}
			for start := 0; ; {
				i := strings.Index(text[start:], v)
				if i < 0 {
					break
				}
				s := start + i
				key := fmt.Sprintf("%d:%d:%s", s, s+len(v), e.Type)
				if !seen[key] {
					seen[key] = true
					out = append(out, Residual{
						Type: e.Type, Value: v,
						Start: doc.RuneOffset(s), End: doc.RuneOffset(s + len(v)),
					})
				}
				start = s + len(v)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	return out
}
