// Package config handles loading and merging of YAML configuration.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/martianzhang/apimart-cli/internal/types"
	"github.com/spf13/viper"
)

const (
	configDir  = ".config/apimart"
	configFile = "config"
	configExt  = "yaml"
	envPrefix  = "APIMART"
)

// Load reads the YAML config from the standard path (~/.config/apimart/config.yaml)
// or a custom path, and returns the parsed Config.
func Load(customPath string) (*types.Config, error) {
	v := viper.New()
	v.SetEnvPrefix(envPrefix)
	v.AutomaticEnv()

	// Bind well-known env vars to config keys
	_ = v.BindEnv("api_key", "APIMART_API_KEY")
	_ = v.BindEnv("base_url", "APIMART_API_BASE")
	_ = v.BindEnv("http_proxy", "APIMART_HTTP_PROXY")

	if customPath != "" {
		v.SetConfigFile(customPath)
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("cannot determine home directory: %w", err)
		}
		v.AddConfigPath(filepath.Join(home, configDir))
		v.SetConfigName(configFile)
	}

	// Ignore "not found" — config is optional
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			// Real error (permission, parse), not just missing
			return nil, fmt.Errorf("error reading config: %w", err)
		}
	}

	cfg := &types.Config{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("error unmarshalling config: %w", err)
	}

	return cfg, nil
}

// LoadDefaults extracts the full Config from the config file.
func LoadDefaults(customPath string) (*types.Config, error) {
	return Load(customPath)
}
