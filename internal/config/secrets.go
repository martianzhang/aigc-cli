package config

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/martianzhang/aigc-cli/internal/secret"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// ErrDecrypt marks a config load failure caused by an encrypted value that
// could not be decrypted — usually a missing or wrong master key. Callers can
// use it to fail loudly instead of silently ignoring a broken config.
var ErrDecrypt = errors.New("config decryption failed")

// SecretLeafNames lists the YAML keys whose values are encrypted at rest. The
// match is by leaf name at any depth, covering api_key (global, providers.*,
// web_search.*) and api_secret.
var SecretLeafNames = map[string]bool{
	"api_key":    true,
	"api_secret": true,
}

// decryptConfigSecrets replaces enc:v1: values in known secret fields with
// their plaintext. Plain values pass through untouched, so plaintext configs
// keep working.
func decryptConfigSecrets(cfg *types.Config) error {
	if cfg == nil {
		return nil
	}
	var err error
	if cfg.APIKey, err = decryptValue("api_key", cfg.APIKey); err != nil {
		return err
	}
	for name, p := range cfg.Providers {
		if p == nil {
			continue
		}
		if p.APIKey, err = decryptValue("providers."+name+".api_key", p.APIKey); err != nil {
			return err
		}
	}
	for name, p := range cfg.WebSearch {
		if p == nil {
			continue
		}
		if p.APIKey, err = decryptValue("web_search."+name+".api_key", p.APIKey); err != nil {
			return err
		}
	}
	return nil
}

// decryptValue decrypts one secret field, leaving plaintext alone.
func decryptValue(label, value string) (string, error) {
	if !secret.IsEncrypted(value) {
		return value, nil
	}
	plaintext, err := secret.DecryptString(value)
	if err != nil {
		return "", fmt.Errorf("%w: %s (%v); set %s or restore the original key",
			ErrDecrypt, label, err, secret.EnvVar)
	}
	return plaintext, nil
}

// EncryptSecretsInFile encrypts every plaintext secret value in the YAML file
// at path, preserving comments and key order. It returns the number of values
// encrypted (0 when the file is missing or already clean) and writes without a
// plaintext backup, since backing up a credential would defeat the point.
func EncryptSecretsInFile(path string) (int, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	doc, err := LoadNode(path)
	if err != nil {
		return 0, err
	}
	count, err := encryptNodeSecrets(doc)
	if err != nil {
		return 0, err
	}
	if count == 0 {
		return 0, nil
	}
	if err := saveNode(path, doc, false); err != nil {
		return 0, err
	}
	return count, nil
}

// encryptNodeSecrets walks a YAML node tree and encrypts every scalar whose key
// is a known secret leaf. Returns the number of values encrypted.
func encryptNodeSecrets(node *yaml.Node) (int, error) {
	count := 0
	var walk func(*yaml.Node) error
	walk = func(n *yaml.Node) error {
		if n == nil {
			return nil
		}
		if n.Kind == yaml.MappingNode {
			for i := 0; i+1 < len(n.Content); i += 2 {
				keyNode, valueNode := n.Content[i], n.Content[i+1]
				if valueNode.Kind == yaml.ScalarNode &&
					SecretLeafNames[keyNode.Value] &&
					valueNode.Value != "" &&
					!secret.IsEncrypted(valueNode.Value) {
					encrypted, err := secret.EncryptString(valueNode.Value)
					if err != nil {
						return err
					}
					valueNode.Value = encrypted
					valueNode.Tag = "!!str"
					valueNode.Style = 0
					count++
				}
				if err := walk(valueNode); err != nil {
					return err
				}
			}
			return nil
		}
		for _, child := range n.Content {
			if err := walk(child); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(node); err != nil {
		return 0, err
	}
	return count, nil
}
