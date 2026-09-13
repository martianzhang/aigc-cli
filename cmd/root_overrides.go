package cmd

import (
	"fmt"
	"net/url"
	"os"
	"reflect"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// maskBaseURL shows just the host part of a URL for display.
func maskBaseURL(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if u.Port() != "" {
		return u.Host
	}
	return u.Host
}

// applyCLIOverrides reflects config defaults yaml tags to auto-discover CLI flag
// mappings, so adding a new field to any *Defaults struct is automatically picked up.
func applyCLIOverrides(cmd *cobra.Command, defaults *types.ConfigDefaults) map[string]string {
	overrides := make(map[string]string)
	if defaults == nil {
		return overrides
	}

	// Ensure all default sections are initialized
	if defaults.Image == nil {
		defaults.Image = &types.ImageDefaults{}
	}
	if defaults.Video == nil {
		defaults.Video = &types.VideoDefaults{}
	}
	if defaults.Midjourney == nil {
		defaults.Midjourney = &types.MidjourneyDefaults{}
	}
	if defaults.Music == nil {
		defaults.Music = &types.MusicDefaults{}
	}
	if defaults.Chat == nil {
		defaults.Chat = &types.ChatDefaults{}
	}

	sub := cmd.Name()
	parent := ""
	if cmd.Parent() != nil {
		parent = cmd.Parent().Name()
	}
	isMJ := parent == "midjourney" || parent == "mj" ||
		sub == "midjourney" || sub == "mj"
	isMusic := parent == "music" || sub == "music"
	fs := cmd.Flags()
	inh := cmd.InheritedFlags()

	switch {
	case sub == "image":
		overrideStruct(fs, inh, defaults.Image, "image", overrides)
	case sub == "video" && !isMJ:
		overrideStruct(fs, inh, defaults.Video, "video", overrides)
	case sub == "chat":
		overrideStruct(fs, inh, defaults.Chat, "chat", overrides)
	case isMJ:
		overrideStruct(fs, inh, defaults.Midjourney, "midjourney", overrides)
	case isMusic:
		overrideStruct(fs, inh, defaults.Music, "music", overrides)
	default:
		// Root command: apply --model to all applicable sections
		for _, s := range []struct {
			ptr interface{}
			key string
		}{
			{defaults.Image, "image"},
			{defaults.Video, "video"},
			{defaults.Chat, "chat"},
		} {
			overrideStruct(fs, inh, s.ptr, s.key, overrides)
		}
	}

	return overrides
}

// overrideStruct uses reflection + yaml tags to auto-discover which CLI flags
// map to struct fields, then overrides matching fields when the flag was set.
//
// Convention: yaml tag "foo_bar"  →  flag name "foo-bar".
// Known mismatches (flag ≠ yaml) are handled via manualAliases.
func overrideStruct(fs, inherited *pflag.FlagSet, structPtr interface{}, section string, overrides map[string]string) {
	v := reflect.ValueOf(structPtr).Elem()
	t := v.Type()

	// Build flag-name → field-index map from yaml tags
	fieldIdx := make(map[string]int)
	yamlNameOf := make(map[int]string)
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("yaml")
		if tag == "" || tag == "-" {
			continue
		}
		yamlName := strings.Split(tag, ",")[0]
		flagName := strings.ReplaceAll(yamlName, "_", "-")
		fieldIdx[flagName] = i
		yamlNameOf[i] = yamlName
	}

	// Manual aliases for known flag-name ↔ yaml-field mismatches
	manualAliases := map[string]string{
		"image-url": "image-urls", // flag is singular, yaml is plural
	}
	for alias, target := range manualAliases {
		if idx, ok := fieldIdx[target]; ok {
			fieldIdx[alias] = idx
		}
	}

	// Determine the best FlagSet for each flag name
	chooseFS := func(name string) *pflag.FlagSet {
		if fs != nil && fs.Changed(name) {
			return fs
		}
		if inherited != nil && inherited.Changed(name) {
			return inherited
		}
		return nil
	}

	for flagName, idx := range fieldIdx {
		source := chooseFS(flagName)
		if source == nil {
			continue
		}

		field := v.Field(idx)
		if !field.IsValid() || !field.CanSet() {
			continue
		}

		origStr := fmt.Sprintf("%v", field.Interface())

		switch field.Kind() {
		case reflect.String:
			if val, err := source.GetString(flagName); err == nil && val != "" && val != origStr {
				field.SetString(val)
			}
		case reflect.Pointer:
			switch field.Type().Elem().Kind() {
			case reflect.Int:
				if val, err := source.GetInt(flagName); err == nil {
					ptr := reflect.New(field.Type().Elem())
					ptr.Elem().SetInt(int64(val))
					field.Set(ptr)
				}
			}
		case reflect.Slice:
			if field.Type().Elem().Kind() == reflect.String {
				if val, err := source.GetStringArray(flagName); err == nil && len(val) > 0 {
					field.Set(reflect.ValueOf(val))
				}
			}
		}

		newStr := fmt.Sprintf("%v", field.Interface())
		if origStr == newStr {
			continue
		}
		if origStr != "" && origStr != "<nil>" && origStr != "[]" {
			key := section + "." + yamlNameOf[idx]
			overrides[key] = fmt.Sprintf("--%s overrides config (orig: %s)", flagName, origStr)
		}
	}
}

// validateCmdProviders checks that all defaults.{cmd}.provider references
// point to existing entries in the providers map. Returns on first error.
// Uses options.CmdProviderMap to keep the command list in one place.
func validateCmdProviders(cfg *types.Config) {
	if cfg == nil || cfg.Defaults == nil {
		return
	}
	for cmdName, info := range options.CmdProviderMap {
		providerRef, _ := info.Getter(cfg.Defaults)
		if providerRef == "" {
			continue
		}
		if err := provider.ValidateProviderRef(providerRef, cfg.Providers); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: defaults.%s.provider: %v\n", cmdName, err)
		}
	}
}
