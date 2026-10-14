// Package config handles loading and merging of YAML configuration.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/martianzhang/aigc-cli/internal/types"
	"github.com/spf13/viper"
)

const (
	configDir  = ".config/aigc-cli"
	configFile = "config"
)

// Load reads the YAML config from ~/.config/aigc-cli/config.yaml or a custom path.
func Load(customPath string) (*types.Config, error) {
	v := viper.New()

	// Bind well-known env vars.  viper.BindEnv almost never fails;
	// errors would only arise from typos in key names, which we catch
	// in development via go vet.
	_ = v.BindEnv("api_key", "OPENAI_API_KEY")
	_ = v.BindEnv("base_url", "OPENAI_BASE_URL")
	_ = v.BindEnv("http_proxy", "HTTP_PROXY", "HTTPS_PROXY")

	if customPath != "" {
		v.SetConfigFile(customPath)
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("cannot determine home directory: %w", err)
		}
		// Default config path
		v.AddConfigPath(filepath.Join(home, ".config", "aigc-cli"))
		v.SetConfigName(configFile)
	}

	// A missing file is optional: continue with env-var (and default) values.
	// This covers both the default search path (ConfigFileNotFoundError) and an
	// explicitly passed --config path (fs.ErrNotExist), so OPENAI_API_KEY /
	// OPENAI_BASE_URL are never silently dropped. Other read errors (permission,
	// parse) are real and surface to the caller.
	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) && !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("error reading config: %w", err)
		}
	}

	cfg := &types.Config{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("error unmarshalling config: %w", err)
	}

	// Decrypt enc:v1: secrets in place so callers always see plaintext.
	if err := decryptConfigSecrets(cfg); err != nil {
		return nil, err
	}

	// Set default API base if not configured
	if cfg.BaseURL == "" {
		cfg.BaseURL = types.DefaultAPIBaseURL
	}

	return cfg, nil
}

// LoadDefaults extracts the full Config from the config file.
func LoadDefaults(customPath string) (*types.Config, error) {
	return Load(customPath)
}
