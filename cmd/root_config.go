package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/martianzhang/aigc-cli/internal/config"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// runPrintConfig prints the effective configuration with inline annotations.
func runPrintConfig(cmd *cobra.Command) {
	cfg, cfgErr := config.Load(shared.CfgFile)
	configFound := cfgErr == nil

	// Track which persistent flags were explicitly set.
	shared.APIKeySet = hasFlagChanged(cmd, "api-key")
	shared.APIBaseSet = hasFlagChanged(cmd, "api-base")
	shared.ProviderSet = hasFlagChanged(cmd, "provider")

	// Populate shared state so ResolveProvider works correctly.
	if cfg != nil {
		shared.Cfg = cfg
		if shared.APIKey == "" {
			shared.APIKey = cfg.APIKey
		}
		if shared.APIBase == "" {
			shared.APIBase = cfg.BaseURL
		}
		if shared.HTTPProxy == "" {
			shared.HTTPProxy = cfg.HTTPProxy
		}
	}

	// Merge config file with env/CLI overrides, then mask every secret before
	// display. Masking runs on a deep copy so shared.Cfg stays intact.
	raw := types.Config{}
	if configFound && cfg != nil {
		raw = *cfg
	}
	// env var / CLI flag takes priority over config file
	if shared.APIKey != "" {
		raw.APIKey = shared.APIKey
	}
	if shared.APIBase != "" && raw.BaseURL == "" {
		raw.BaseURL = shared.APIBase
	}
	if shared.HTTPProxy != "" && raw.HTTPProxy == "" {
		raw.HTTPProxy = shared.HTTPProxy
	}
	displayCfg := &configDisplay{}
	if masked := service.MaskConfigSecrets(&raw); masked != nil {
		displayCfg.Config = *masked
	}
	if displayCfg.Defaults == nil {
		displayCfg.Defaults = &types.ConfigDefaults{}
	}
	if displayCfg.Defaults.Image == nil {
		displayCfg.Defaults.Image = &types.ImageDefaults{}
	}

	// Apply CLI flag overrides to config defaults — shows effective configuration
	overrides := applyCLIOverrides(cmd, displayCfg.Defaults)

	// Build annotations
	var configNote, proxyNote string

	// Config file path
	if shared.CfgFile != "" {
		configNote = fmt.Sprintf("# config: %s (explicit)", shared.CfgFile)
	} else if configFound {
		if cfg != nil {
			home, _ := os.UserHomeDir()
			candidates := []string{
				filepath.Join(home, ".config", "aigc-cli", "config.yaml"),
			}
			for _, p := range candidates {
				if _, err := os.Stat(p); err == nil {
					configNote = fmt.Sprintf("# config: %s", p)
					break
				}
			}
		}
		if configNote == "" {
			configNote = "# config: found"
		}
	} else {
		configNote = "# config: not found (env vars / code defaults)"
	}

	// Proxy
	if displayCfg.HTTPProxy != "" {
		proxyNote = fmt.Sprintf("# http_proxy: %s", displayCfg.HTTPProxy)
	} else if envProxy := os.Getenv("HTTP_PROXY"); envProxy != "" {
		proxyNote = ""
	} else {
		proxyNote = "# http_proxy: not set (direct connection)"
	}

	b, _ := yaml.Marshal(displayCfg)
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")

	// Print YAML with inline annotations
	defaultsSection := ""
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		indent := len(line) - len(strings.TrimLeft(line, " "))
		key := strings.SplitN(trimmed, ":", 2)[0]

		// Track YAML section context for nested override annotations
		if trimmed == "defaults:" {
			defaultsSection = "root"
		} else if defaultsSection != "" && indent == 4 && strings.HasSuffix(trimmed, ":") {
			defaultsSection = strings.TrimSuffix(trimmed, ":")
		} else if indent < 2 {
			defaultsSection = ""
		}

		var annotation string
		switch key {
		default:
			// Generic override annotation for any defaults field
			if defaultsSection != "" {
				if note, ok := overrides[defaultsSection+"."+key]; ok {
					annotation = note
				}
			}
		}
		if annotation != "" {
			fmt.Printf("%s  # %s\n", line, annotation)
		} else {
			fmt.Println(line)
		}
	}

	// Append standalone annotations
	for _, n := range []string{configNote, proxyNote} {
		if n != "" {
			fmt.Println(n)
		}
	}
}
