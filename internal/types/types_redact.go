package types

// RedactConfig configures the `redact` command. It is the defaults.redact
// section of config.yaml: provider/model route the optional model request,
// while the remaining fields control rules, masking and engine selection.
type RedactConfig struct {
	// Provider and Model select the named provider and model for engine=model|auto.
	Provider string `mapstructure:"provider" yaml:"provider,omitempty"`
	Model    string `mapstructure:"model" yaml:"model,omitempty"`

	// ConfigFile points to a standalone redact YAML (relative to config.yaml).
	// It is overlaid on top of this section, so a profile can keep only the
	// fields it changes. Profile selects an entry from Profiles; --profile and
	// --redact-config override both for a single run.
	ConfigFile string            `mapstructure:"config_file" yaml:"config_file,omitempty"`
	Profile    string            `mapstructure:"profile" yaml:"profile,omitempty"`
	Profiles   map[string]string `mapstructure:"profiles" yaml:"profiles,omitempty"`

	// Style is the default replacement style: placeholder | mask | black | hash.
	Style string `mapstructure:"style" yaml:"style,omitempty"`
	// Placeholder is the template used by style=placeholder. It must contain
	// {type} and {n}, e.g. "<{type}_{n}>".
	Placeholder string `mapstructure:"placeholder" yaml:"placeholder,omitempty"`
	// Engine selects rules | model | auto (auto = rules + model when configured).
	Engine string `mapstructure:"engine" yaml:"engine,omitempty"`
	// Date and Amount opt semantic entities into replacement. They are off by
	// default because changing dates/amounts changes the meaning of a document.
	Date   bool `mapstructure:"date" yaml:"date,omitempty"`
	Amount bool `mapstructure:"amount" yaml:"amount,omitempty"`

	// Rules toggles built-in rules by name. Missing entries keep the built-in
	// default. An unknown name is a configuration error.
	Rules map[string]bool `mapstructure:"rules" yaml:"rules,omitempty"`
	// Overrides patches built-in rules without copying their regex.
	Overrides map[string]*RedactRulePatch `mapstructure:"overrides" yaml:"overrides,omitempty"`
	// Custom appends user-defined rules. A name that collides with a built-in
	// rule is rejected; use Overrides instead.
	Custom []RedactRule `mapstructure:"custom" yaml:"custom,omitempty"`

	// Allow lists exact strings that must not be redacted. Deny lists strings
	// that must always be redacted. A value in both lists is an error.
	Allow []string `mapstructure:"allow" yaml:"allow,omitempty"`
	Deny  []string `mapstructure:"deny" yaml:"deny,omitempty"`
	// Aliases groups spellings of the same entity so they share a placeholder.
	Aliases map[string][]string `mapstructure:"aliases" yaml:"aliases,omitempty"`
	// Dictionary lists exact terms (organizations, projects, ...) that are
	// always redacted, even without a model. Small lists can stay inline;
	// larger lists belong in dictionary_files.
	Dictionary []RedactDictionaryEntry `mapstructure:"dictionary" yaml:"dictionary,omitempty"`
	// DictionaryFiles are TSV/CSV/JSON files loaded relative to the config
	// file's directory. TSV is recommended for large human-edited lists.
	DictionaryFiles []string `mapstructure:"dictionary_files" yaml:"dictionary_files,omitempty"`

	Mask *RedactMaskPolicy `mapstructure:"mask" yaml:"mask,omitempty"`

	// ModelEngine selects auto | chatner | tokencls. auto prefers tokencls when
	// tokencls is configured (or the provider is type=local), else chatner.
	ModelEngine string                `mapstructure:"model_engine" yaml:"model_engine,omitempty"`
	Chatner     *RedactChatnerConfig  `mapstructure:"chatner" yaml:"chatner,omitempty"`
	Tokencls    *RedactTokenclsConfig `mapstructure:"tokencls" yaml:"tokencls,omitempty"`
}

// RedactRule is one user-defined rule (or a built-in rule exported by
// `redact rules --export`).
type RedactRule struct {
	Name      string `mapstructure:"name" yaml:"name"`
	Category  string `mapstructure:"category" yaml:"category,omitempty"`
	Type      string `mapstructure:"type" yaml:"type,omitempty"`
	Regex     string `mapstructure:"regex" yaml:"regex"`
	Style     string `mapstructure:"style" yaml:"style,omitempty"`
	Validator string `mapstructure:"validator" yaml:"validator,omitempty"`
	Boundary  string `mapstructure:"boundary" yaml:"boundary,omitempty"`
	// Group selects the capture group to redact (0 = whole match).
	Group    int `mapstructure:"group" yaml:"group,omitempty"`
	Priority int `mapstructure:"priority" yaml:"priority,omitempty"`
	MaxLen   int `mapstructure:"max_len" yaml:"max_len,omitempty"`
	// Enabled defaults to true when nil.
	Enabled *bool `mapstructure:"enabled" yaml:"enabled,omitempty"`
	// Mask overrides the global mask policy for this rule.
	Mask *RedactMaskPolicy `mapstructure:"mask" yaml:"mask,omitempty"`
}

// RedactRulePatch changes selected fields of a built-in rule.
type RedactRulePatch struct {
	Regex     string            `mapstructure:"regex" yaml:"regex,omitempty"`
	Style     string            `mapstructure:"style" yaml:"style,omitempty"`
	Validator string            `mapstructure:"validator" yaml:"validator,omitempty"`
	Boundary  string            `mapstructure:"boundary" yaml:"boundary,omitempty"`
	Group     *int              `mapstructure:"group" yaml:"group,omitempty"`
	Priority  *int              `mapstructure:"priority" yaml:"priority,omitempty"`
	MaxLen    *int              `mapstructure:"max_len" yaml:"max_len,omitempty"`
	Enabled   *bool             `mapstructure:"enabled" yaml:"enabled,omitempty"`
	Mask      *RedactMaskPolicy `mapstructure:"mask" yaml:"mask,omitempty"`
}

// RedactMaskPolicy controls how a value is masked. Strategies provide sensible
// per-type defaults (phone keeps 3+4, ID card keeps 6+4, etc.).
type RedactMaskPolicy struct {
	// Strategy is fixed | name | email | address | phone | id_card | bank_card.
	Strategy string `mapstructure:"strategy" yaml:"strategy,omitempty"`
	// Position is middle | prefix | suffix | all.
	Position string `mapstructure:"position" yaml:"position,omitempty"`
	// KeepPrefix/KeepSuffix count characters (runes) to keep.
	KeepPrefix int `mapstructure:"keep_prefix" yaml:"keep_prefix,omitempty"`
	KeepSuffix int `mapstructure:"keep_suffix" yaml:"keep_suffix,omitempty"`
	// Regex anchors take precedence over the numeric counts when they match.
	KeepPrefixRegex string `mapstructure:"keep_prefix_regex" yaml:"keep_prefix_regex,omitempty"`
	KeepSuffixRegex string `mapstructure:"keep_suffix_regex" yaml:"keep_suffix_regex,omitempty"`
	Char            string `mapstructure:"char" yaml:"char,omitempty"`
	MaskLen         int    `mapstructure:"mask_len" yaml:"mask_len,omitempty"`
	// PreserveLength defaults to true (one mask char per masked rune).
	PreserveLength *bool `mapstructure:"preserve_length" yaml:"preserve_length,omitempty"`
	// KeepSeparators defaults to true (spaces, dashes, dots stay readable).
	KeepSeparators *bool `mapstructure:"keep_separators" yaml:"keep_separators,omitempty"`
	// Overrides applies per entity type on top of the global policy.
	Overrides map[string]*RedactMaskPolicy `mapstructure:"overrides" yaml:"overrides,omitempty"`
}

// RedactTokenclsConfig configures a local ONNX token-classification model.
type RedactTokenclsConfig struct {
	// ModelDir is an explicit directory containing model.onnx, config.json and
	// tokenizer.json. When empty, models_dir/redact/<model> is used.
	ModelDir string `mapstructure:"model_dir" yaml:"model_dir,omitempty"`
	Model    string `mapstructure:"model" yaml:"model,omitempty"`
	// MaxLen is the padded sequence length (default 512), Stride the window step.
	MaxLen int `mapstructure:"max_len" yaml:"max_len,omitempty"`
	Stride int `mapstructure:"stride" yaml:"stride,omitempty"`
	// Inputs overrides auto-detected ONNX input names.
	Inputs []string `mapstructure:"inputs" yaml:"inputs,omitempty"`
	// Output overrides the ONNX output name (default: logits, output, emissions).
	Output string `mapstructure:"output" yaml:"output,omitempty"`
	// OperatingPoint selects the BIOES transition biases: high_recall
	// (default, matches privyscope zh-Hans), balanced or high_precision.
	OperatingPoint string `mapstructure:"operating_point" yaml:"operating_point,omitempty"`
	// Biases overrides OperatingPoint with six raw values in category order:
	// outside, begin, single, inside, end, exit.
	Biases []float64 `mapstructure:"biases" yaml:"biases,omitempty"`
}

// RedactDictionaryEntry is one exact term that should always be redacted.
type RedactDictionaryEntry struct {
	Value string `mapstructure:"value" yaml:"value"`
	Type  string `mapstructure:"type" yaml:"type,omitempty"`
}

// RedactChatnerConfig configures a value-level, OpenAI-compatible NER model.
// HaS 4.0 is the reference preset, but any model returning JSON entity lists
// can be used by changing provider/model/prompt.
type RedactChatnerConfig struct {
	Occurrence   string   `mapstructure:"occurrence" yaml:"occurrence,omitempty"` // anchored | all | first
	AnchorWindow int      `mapstructure:"anchor_window" yaml:"anchor_window,omitempty"`
	Types        []string `mapstructure:"types" yaml:"types,omitempty"`
	Prompt       string   `mapstructure:"prompt" yaml:"prompt,omitempty"`
}

// RedactDefaults is the defaults.redact section. It shares RedactConfig so the
// provider/model selection and the command behavior live in one place.
type RedactDefaults = RedactConfig
