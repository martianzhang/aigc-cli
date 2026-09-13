package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/martianzhang/aigc-cli/internal/config"
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

	displayCfg := &configDisplay{}
	if configFound && cfg != nil {
		displayCfg.Config = *cfg
	}
	// env var / CLI flag takes priority over config file
	if shared.APIKey != "" {
		displayCfg.APIKey = shared.APIKey
	}
	if shared.APIBase != "" && displayCfg.BaseURL == "" {
		displayCfg.BaseURL = shared.APIBase
	}
	if shared.HTTPProxy != "" && displayCfg.HTTPProxy == "" {
		displayCfg.HTTPProxy = shared.HTTPProxy
	}
	// Mask API key for display
	if displayCfg.APIKey != "" {
		if len(displayCfg.APIKey) > 8 {
			displayCfg.APIKey = displayCfg.APIKey[:8] + "..."
		} else if len(displayCfg.APIKey) > 3 {
			displayCfg.APIKey = displayCfg.APIKey[:3] + "..."
		} else {
			displayCfg.APIKey = displayCfg.APIKey[:1] + "..."
		}
	}
	// Mask provider API keys
	for _, p := range displayCfg.Providers {
		if p != nil && p.APIKey != "" {
			if len(p.APIKey) > 8 {
				p.APIKey = p.APIKey[:8] + "..."
			} else if len(p.APIKey) > 3 {
				p.APIKey = p.APIKey[:3] + "..."
			} else {
				p.APIKey = p.APIKey[:1] + "..."
			}
		}
	}
	// Mask web search provider API keys
	for _, p := range displayCfg.WebSearch {
		if p != nil && p.APIKey != "" {
			if len(p.APIKey) > 8 {
				p.APIKey = p.APIKey[:8] + "..."
			} else if len(p.APIKey) > 3 {
				p.APIKey = p.APIKey[:3] + "..."
			} else {
				p.APIKey = p.APIKey[:1] + "..."
			}
		}
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

	// ── 各命令有效 Provider 概览 ──
	fmt.Println()
	printCmdProviders()
	fmt.Println()
}

// printCmdProviders prints the effective provider for each command.
func printCmdProviders() {
	cmds := []struct {
		Name string
		Ref  string
	}{
		{"image", ProviderNameImage},
		{"video", ProviderNameVideo},
		{"chat", ProviderNameChat},
		{"audio", ProviderNameAudio},
		{"midjourney", ProviderNameMidjourney},
		{"music", ProviderNameMusic},
		{"ocr", ProviderNameOCR},
		{"vision", ProviderNameVision},
		{"detect", ProviderNameDetect},
		{"background", ProviderNameBackground},
	}

	fmt.Println("# ── 各命令有效 Provider ──")
	for _, c := range cmds {
		p := shared.ResolveProvider(c.Ref)
		if p == nil {
			continue
		}
		providerLabel := p.Name
		if providerLabel == "" {
			providerLabel = "(global)"
		}
		// Show explicit type if set, otherwise fall back to URL-based detection.
		typeLabel := p.Type.DisplayType(p.ProviderType.String())
		modelInfo := ""
		if p.Model != "" {
			modelInfo = fmt.Sprintf(", model=%s", p.Model)
		}
		fmt.Printf("  %-12s → %s (%s%s) %s\n",
			c.Name+":", providerLabel, typeLabel, modelInfo, maskBaseURL(p.BaseURL))
	}
}
