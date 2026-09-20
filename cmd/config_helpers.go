package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/martianzhang/aigc-cli/internal/config"
	"github.com/martianzhang/aigc-cli/internal/service"
)

// secretKeyNames lists config keys whose value must never be printed in full.
var secretKeyNames = map[string]bool{
	"api_key":    true,
	"api_secret": true,
}

// urlKeyNames lists config keys that may hide credentials inside a URL.
var urlKeyNames = map[string]bool{
	"base_url":   true,
	"http_proxy": true,
}

// configFilePath resolves the config file exactly like the loader does:
// --config (shared.CfgFile) first, then the default path.
func configFilePath() (string, error) {
	if shared.CfgFile != "" {
		return shared.CfgFile, nil
	}
	return config.DefaultPath()
}

// loadExistingConfig fails with a clear message when the file is missing:
// get/set never create it.
func loadExistingConfig(path string) (*yaml.Node, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("config file not found: %s (create it first; see docs/config.example.yaml)", path)
		}
		return nil, fmt.Errorf("config file: %w", err)
	}
	return config.LoadNode(path)
}

// isSensitiveConfigKey reports whether writing this dot path changes a
// credential or an endpoint override, which requires --force.
func isSensitiveConfigKey(dotPath string) bool {
	leaf := lastPathSegment(dotPath)
	return secretKeyNames[leaf] || urlKeyNames[leaf]
}

// maskConfigLeaf masks one value according to the key it belongs to.
func maskConfigLeaf(dotPath, value string) string {
	if value == "" {
		return value
	}
	leaf := lastPathSegment(dotPath)
	if secretKeyNames[leaf] {
		return service.MaskKey(value)
	}
	if urlKeyNames[leaf] {
		return service.RedactURLSecrets(value)
	}
	return value
}

// lastPathSegment returns the final dot-path segment.
func lastPathSegment(dotPath string) string {
	segments := strings.Split(dotPath, ".")
	return segments[len(segments)-1]
}

// printConfigNode renders a `config get` result: scalars print raw, sections
// print YAML with every nested secret masked.
func printConfigNode(w io.Writer, dotPath string, node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		fmt.Fprintln(w, maskConfigLeaf(dotPath, node.Value))
		return nil
	}
	maskNodeSecrets(node)
	data, err := config.MarshalNode(node, 2)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

// maskNodeSecrets walks a node tree and masks every value whose key is a
// known secret, so `config get providers` cannot leak credentials.
func maskNodeSecrets(node *yaml.Node) {
	if node == nil {
		return
	}
	if node.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(node.Content); i += 2 {
			keyNode, valueNode := node.Content[i], node.Content[i+1]
			maskNodeSecrets(valueNode)
			if valueNode.Kind == yaml.ScalarNode && valueNode.Value != "" {
				valueNode.Value = maskConfigLeaf(keyNode.Value, valueNode.Value)
			}
		}
		return
	}
	for _, child := range node.Content {
		maskNodeSecrets(child)
	}
}
