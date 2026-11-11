package redact

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/martianzhang/aigc-cli/internal/types"
)

// MaskPolicy is the compiled, internal form of types.RedactMaskPolicy.
type MaskPolicy struct {
	Strategy        string
	Position        string
	KeepPrefix      int
	KeepSuffix      int
	KeepPrefixRegex string
	KeepSuffixRegex string
	Char            string
	MaskLen         int
	PreserveLength  bool
	KeepSeparators  bool

	prefixRe *regexp.Regexp
	suffixRe *regexp.Regexp
}

var compoundSurnames = []string{
	"欧阳", "司马", "上官", "诸葛", "东方", "皇甫", "尉迟", "公孙",
	"慕容", "司徒", "令狐", "夏侯", "宇文", "长孙", "轩辕", "端木",
	"百里", "东郭", "南宫", "西门", "独孤", "申屠", "呼延", "钟离",
	"梁丘", "左丘", "东门", "第五",
}

// defaultMaskPolicy returns the built-in policy for an entity type.
func defaultMaskPolicy(typ string) MaskPolicy {
	p := MaskPolicy{
		Strategy: "fixed", Position: "middle",
		KeepPrefix: 3, KeepSuffix: 4,
		Char: "*", MaskLen: 3,
		PreserveLength: true, KeepSeparators: true,
	}
	switch strings.ToUpper(typ) {
	case "ID_CARD":
		p.Strategy, p.KeepPrefix, p.KeepSuffix = "id_card", 6, 4
	case "PHONE":
		p.Strategy, p.KeepPrefix, p.KeepSuffix = "phone", 3, 4
	case "BANK_CARD":
		p.Strategy, p.Position, p.KeepPrefix, p.KeepSuffix = "bank_card", "prefix", 0, 4
	case "EMAIL":
		p.Strategy, p.Position, p.KeepPrefix = "email", "middle", 1
		p.KeepSuffixRegex = "@.*$"
	case "PERSON":
		// Keep the surname/initial but always use one star for the given name,
		// so 张三 and 张三丰 look identical and length is not leaked.
		p.Strategy, p.PreserveLength, p.MaskLen = "name", false, 1
	case "ADDRESS":
		p.Strategy, p.Position = "address", "suffix"
		p.KeepPrefixRegex = `^.*?(?:省|市|区|县|镇|街道|乡)`
	case "ORG":
		p.Position = "all"
	case "PASSWORD":
		p.Position, p.PreserveLength, p.MaskLen = "all", false, 8
	case "USERNAME":
		p.Position, p.KeepPrefix, p.KeepSuffix = "middle", 2, 2
	case "DB_URL":
		p.Position, p.PreserveLength, p.MaskLen = "all", false, 12
	case "HOST":
		p.Position, p.KeepPrefix, p.KeepSuffix = "middle", 3, 4
	case "CREDENTIAL", "COOKIE", "API_KEY", "TOKEN", "PRIVATE_KEY":
		p.Position, p.PreserveLength, p.MaskLen = "all", false, 8
	case "URL":
		p.Position, p.PreserveLength, p.MaskLen = "all", false, 12
	case "CRYPTO":
		p.Position, p.KeepPrefix, p.KeepSuffix = "middle", 4, 4
	case "WORK":
		p.Position, p.PreserveLength, p.MaskLen = "all", false, 8
	case "ETHNICITY":
		// Keep only the generic 族 suffix; the number of stars must not reveal
		// which ethnicity it was.
		p.Position, p.KeepSuffix, p.PreserveLength, p.MaskLen = "prefix", 1, false, 1
	case "DATE":
		p.Strategy = "date"
	}
	return p
}

// MaskResolver resolves the policy for a type: built-in defaults first, then
// redact.mask.overrides[type]. The global redact.mask section is the fallback
// for custom/unknown types, so writing `keep_prefix: 3` globally does not
// silently weaken the ID-card default of 6.
type MaskResolver struct {
	global MaskPolicy
	byType map[string]MaskPolicy
}

// NewMaskResolver compiles configured policies.
func NewMaskResolver(cfg *types.RedactConfig) (*MaskResolver, error) {
	global := defaultMaskPolicy("")
	if cfg != nil && cfg.Mask != nil {
		applyMaskConfig(&global, cfg.Mask)
	}
	r := &MaskResolver{global: global, byType: map[string]MaskPolicy{}}
	if cfg == nil || cfg.Mask == nil {
		return r, r.compileAll()
	}
	for typ, override := range cfg.Mask.Overrides {
		if override == nil {
			continue
		}
		key := strings.ToUpper(strings.TrimSpace(typ))
		p := defaultMaskPolicy(key)
		applyMaskConfig(&p, override)
		r.byType[key] = p
	}
	return r, r.compileAll()
}

func (r *MaskResolver) compileAll() error {
	if err := compileMask(&r.global); err != nil {
		return err
	}
	for typ, p := range r.byType {
		if err := compileMask(&p); err != nil {
			return fmt.Errorf("mask.overrides.%s: %w", typ, err)
		}
		r.byType[typ] = p
	}
	return nil
}

func compileMask(p *MaskPolicy) error {
	switch p.Strategy {
	case "fixed", "name", "email", "address", "phone", "id_card", "bank_card", "date":
	default:
		return fmt.Errorf("unknown mask strategy %q", p.Strategy)
	}
	switch p.Position {
	case "middle", "prefix", "suffix", "all":
	default:
		return fmt.Errorf("unknown mask position %q", p.Position)
	}
	if p.Char == "" {
		return fmt.Errorf("mask char must not be empty")
	}
	if p.MaskLen <= 0 {
		p.MaskLen = 3
	}
	var err error
	if p.KeepPrefixRegex != "" {
		p.prefixRe, err = regexp.Compile(p.KeepPrefixRegex)
		if err != nil {
			return fmt.Errorf("invalid keep_prefix_regex: %w", err)
		}
	}
	if p.KeepSuffixRegex != "" {
		p.suffixRe, err = regexp.Compile(p.KeepSuffixRegex)
		if err != nil {
			return fmt.Errorf("invalid keep_suffix_regex: %w", err)
		}
	}
	return nil
}

// Policy returns the compiled policy for typ.
func (r *MaskResolver) Policy(typ string) MaskPolicy {
	key := strings.ToUpper(strings.TrimSpace(typ))
	if p, ok := r.byType[key]; ok {
		return p
	}
	if isBuiltinMaskType(key) {
		p := defaultMaskPolicy(key)
		_ = compileMask(&p)
		return p
	}
	return r.global
}

func isBuiltinMaskType(typ string) bool {
	switch typ {
	case "ID_CARD", "PHONE", "BANK_CARD", "EMAIL", "PERSON", "ADDRESS", "ORG",
		"PASSWORD", "USERNAME", "DB_URL", "HOST", "CREDENTIAL", "COOKIE", "API_KEY",
		"TOKEN", "PRIVATE_KEY", "URL", "CRYPTO", "WORK", "ETHNICITY", "DATE":
		return true
	}
	return false
}

func applyMaskConfig(dst *MaskPolicy, src *types.RedactMaskPolicy) {
	if src.Strategy != "" {
		dst.Strategy = src.Strategy
	}
	if src.Position != "" {
		dst.Position = src.Position
	}
	if src.KeepPrefix != 0 {
		dst.KeepPrefix = src.KeepPrefix
	}
	if src.KeepSuffix != 0 {
		dst.KeepSuffix = src.KeepSuffix
	}
	if src.KeepPrefixRegex != "" {
		dst.KeepPrefixRegex = src.KeepPrefixRegex
	}
	if src.KeepSuffixRegex != "" {
		dst.KeepSuffixRegex = src.KeepSuffixRegex
	}
	if src.Char != "" {
		dst.Char = src.Char
	}
	if src.MaskLen != 0 {
		dst.MaskLen = src.MaskLen
	}
	if src.PreserveLength != nil {
		dst.PreserveLength = *src.PreserveLength
	}
	if src.KeepSeparators != nil {
		dst.KeepSeparators = *src.KeepSeparators
	}
}

// MaskValue masks one entity value according to p.
// BlackValue masks every rune with a block, but keeps the 族 suffix in an
// ethnicity because "█族" is more readable than "██" and still hides the name.
func BlackValue(value, typ string) string {
	runes := []rune(value)
	switch strings.ToUpper(typ) {
	case "PERSON":
		return "██"
	case "ETHNICITY":
		if len(runes) > 1 && runes[len(runes)-1] == '族' {
			return "█族"
		}
		return "█"
	case "DATE":
		return maskDate(value, "█")
	}
	return strings.Repeat("█", len(runes))
}

func MaskValue(value, typ string, p MaskPolicy) string {
	if value == "" {
		return value
	}
	runes := []rune(value)
	n := len(runes)
	if n == 0 {
		return value
	}

	var keepStart, keepEnd int
	switch p.Strategy {
	case "name":
		if isASCIIWords(value) {
			return maskASCIIWords(value, p)
		}
		keepStart, keepEnd = nameKeep(value)
	case "email":
		return maskEmail(value, p)
	case "address":
		keepStart, keepEnd = addressKeep(value, p)
	case "date":
		return maskDate(value, p.Char)
	default:
		switch p.Position {
		case "middle":
			keepStart, keepEnd = p.KeepPrefix, p.KeepSuffix
		case "suffix":
			keepStart, keepEnd = p.KeepPrefix, 0
		case "prefix":
			keepStart, keepEnd = 0, p.KeepSuffix
		case "all":
			keepStart, keepEnd = 0, 0
		}
	}

	if p.prefixRe != nil && p.Strategy != "address" {
		if loc := p.prefixRe.FindStringIndex(value); loc != nil && loc[0] == 0 {
			keepStart = utf8.RuneCountInString(value[:loc[1]])
		}
	}
	if p.suffixRe != nil {
		if loc := p.suffixRe.FindStringIndex(value); loc != nil && loc[1] == len(value) {
			keepEnd = utf8.RuneCountInString(value[loc[0]:])
		}
	}

	// Never let the kept parts swallow the whole value.
	if keepStart < 0 {
		keepStart = 0
	}
	if keepEnd < 0 {
		keepEnd = 0
	}
	if keepStart > n {
		keepStart = n
	}
	if keepEnd > n {
		keepEnd = n
	}
	if keepStart+keepEnd >= n {
		if n == 1 {
			keepStart, keepEnd = 0, 0
		} else {
			keepStart = minInt(keepStart, n-1)
			keepEnd = minInt(keepEnd, n-keepStart-1)
		}
	}

	var b strings.Builder
	b.WriteString(string(runes[:keepStart]))
	if p.PreserveLength {
		masked := runes[keepStart : n-keepEnd]
		for _, r := range masked {
			if p.KeepSeparators && IsSeparator(r) {
				b.WriteRune(r)
				continue
			}
			b.WriteString(p.Char)
		}
	} else {
		b.WriteString(strings.Repeat(p.Char, p.MaskLen))
	}
	b.WriteString(string(runes[n-keepEnd:]))
	return b.String()
}

func nameKeep(value string) (int, int) {
	for _, s := range compoundSurnames {
		if strings.HasPrefix(value, s) {
			return utf8.RuneCountInString(s), 0
		}
	}
	return 1, 0
}

func maskEmail(value string, p MaskPolicy) string {
	at := strings.Index(value, "@")
	if at <= 0 {
		return MaskValue(value, "EMAIL", MaskPolicy{
			Strategy: "fixed", Position: "middle", KeepPrefix: 1, KeepSuffix: 0,
			Char: p.Char, MaskLen: p.MaskLen, PreserveLength: true,
		})
	}
	local := []rune(value[:at])
	var b strings.Builder
	if len(local) <= 1 {
		// A one-character local part is the identity; do not keep it.
		b.WriteString(p.Char)
	} else {
		b.WriteString(string(local[0]))
		maskLen := len(local) - 1
		if !p.PreserveLength && p.MaskLen > 0 {
			maskLen = p.MaskLen
		}
		b.WriteString(strings.Repeat(p.Char, maskLen))
	}
	b.WriteString(value[at:])
	return b.String()
}

func addressKeep(value string, p MaskPolicy) (int, int) {
	markers := []string{"街道", "省", "市", "区", "县", "镇", "乡"}
	best := -1
	for _, m := range markers {
		if i := strings.LastIndex(value, m); i >= 0 {
			end := i + len(m)
			if end > best {
				best = end
			}
		}
	}
	if best > 0 {
		return utf8.RuneCountInString(value[:best]), 0
	}
	return p.KeepPrefix, 0
}

func isASCIIWords(s string) bool {
	if s == "" {
		return false
	}
	hasLetter := false
	for _, r := range s {
		if r == ' ' {
			continue
		}
		if r > utf8.RuneSelf {
			return false
		}
		hasLetter = true
	}
	return hasLetter
}

func maskASCIIWords(value string, p MaskPolicy) string {
	words := strings.Fields(value)
	for i, w := range words {
		runes := []rune(w)
		if len(runes) > 1 {
			maskLen := len(runes) - 1
			if !p.PreserveLength && p.MaskLen > 0 {
				maskLen = p.MaskLen
			}
			words[i] = string(runes[0]) + strings.Repeat(p.Char, maskLen)
		}
	}
	return strings.Join(words, " ")
}

// maskDigits replaces every digit with char but keeps units/separators such as
// 年/月/日 and - /.
func maskDigits(value, char string) string {
	var b strings.Builder
	for _, r := range value {
		if unicode.IsDigit(r) {
			b.WriteString(char)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// maskDate masks every digit but keeps the calendar units. Component widths are
// normalized (4/2/2) so 1987年3月12日 and 1979年11月8日 both become
// ****年**月**日; an observer cannot infer the original digit count.
func maskDate(value, char string) string {
	switch {
	case strings.Contains(value, "年"):
		out := strings.Repeat(char, 4) + "年"
		if strings.Contains(value, "月") {
			out += strings.Repeat(char, 2) + "月"
			if strings.Contains(value, "日") {
				out += strings.Repeat(char, 2) + "日"
			}
		}
		return out
	case strings.Contains(value, "-"):
		return maskDateSeparated(value, "-", char)
	case strings.Contains(value, "/"):
		return maskDateSeparated(value, "/", char)
	case strings.Contains(value, "."):
		return maskDateSeparated(value, ".", char)
	}
	return maskDigits(value, char)
}

func maskDateSeparated(value, sep, char string) string {
	parts := strings.Split(value, sep)
	widths := []int{4, 2, 2}
	for i := range parts {
		width := 2
		if i < len(widths) {
			width = widths[i]
		}
		parts[i] = strings.Repeat(char, width)
	}
	return strings.Join(parts, sep)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
