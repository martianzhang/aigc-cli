package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Errors returned by NodeAt and SetScalar. Callers add the command context
// (for example "set defaults.chat.max_iterations: section not found").
var (
	// ErrKeyNotFound means no node exists at the requested dot path.
	ErrKeyNotFound = errors.New("key not found")
	// ErrSectionNotFound means a parent segment of the dot path is missing.
	ErrSectionNotFound = errors.New("section not found")
)

// decimalIntRE matches plain decimal integers without leading zeros, so values
// that only look numeric (padded ids, API keys) stay strings.
var decimalIntRE = regexp.MustCompile(`^[+-]?(0|[1-9][0-9]*)$`)

// DefaultPath returns the config file path used when --config is not given:
// ~/.config/aigc-cli/config.yaml.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, configDir, configFile+".yaml"), nil
}

// LoadNode reads path into a YAML document node. Comments, key order and
// scalar styles are preserved in the node tree for a later SaveNode.
func LoadNode(path string) (*yaml.Node, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	return &doc, nil
}

// NodeAt returns the value node at a dot-separated path (for example
// "defaults.image.model"). Keys are matched literally.
func NodeAt(doc *yaml.Node, dotPath string) (*yaml.Node, error) {
	segments, err := splitPath(dotPath)
	if err != nil {
		return nil, err
	}
	current := rootMapping(doc)
	for i, segment := range segments {
		if current == nil {
			return nil, ErrKeyNotFound
		}
		value := mappingValue(current, segment)
		if value == nil {
			return nil, ErrKeyNotFound
		}
		if i == len(segments)-1 {
			return value, nil
		}
		if value.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("%s is not a section", strings.Join(segments[:i+1], "."))
		}
		current = value
	}
	return nil, ErrKeyNotFound
}

// SetScalar replaces the scalar at dotPath with value. When the key already
// exists its YAML type is kept (int stays int, bool stays bool); a new key is
// created only inside an existing parent section. Missing parents are an
// error: sections are never created implicitly.
func SetScalar(doc *yaml.Node, dotPath, value string) error {
	segments, err := splitPath(dotPath)
	if err != nil {
		return err
	}
	root, err := ensureRootMapping(doc)
	if err != nil {
		return err
	}
	current := root
	for i := 0; i < len(segments)-1; i++ {
		next := mappingValue(current, segments[i])
		if next == nil {
			return ErrSectionNotFound
		}
		if next.Kind != yaml.MappingNode {
			return fmt.Errorf("%s is not a section", strings.Join(segments[:i+1], "."))
		}
		current = next
	}

	leafKey := segments[len(segments)-1]
	leaf := mappingValue(current, leafKey)
	if leaf == nil {
		current.Content = append(current.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: leafKey},
			newScalarNode(value))
		return nil
	}
	return setScalarValue(leaf, value)
}

// setScalarValue keeps node's YAML type and only accepts values that parse as
// that type; strings accept everything.
func setScalarValue(node *yaml.Node, value string) error {
	if node.Kind != yaml.ScalarNode {
		return errors.New("value is not a scalar")
	}
	var (
		normalized string
		err        error
	)
	switch node.Tag {
	case "!!int":
		normalized, err = parseYAMLInt(value)
	case "!!bool":
		normalized, err = parseYAMLBool(value)
	case "!!float":
		normalized, err = parseYAMLFloat(value)
	default:
		node.Tag = "!!str"
		node.Value = value
		return nil
	}
	if err != nil {
		return err
	}
	node.Value = normalized
	return nil
}

// parseYAMLInt accepts anything YAML itself would resolve as an int.
func parseYAMLInt(value string) (string, error) {
	tag, normalized, err := resolveScalar(value)
	if err != nil || tag != "!!int" {
		return "", fmt.Errorf("cannot convert %q to int", value)
	}
	return normalized, nil
}

// parseYAMLBool accepts YAML booleans plus the strconv forms (1/0, t/f).
func parseYAMLBool(value string) (string, error) {
	if tag, normalized, err := resolveScalar(value); err == nil && tag == "!!bool" {
		return normalized, nil
	}
	if parsed, err := strconv.ParseBool(strings.ToLower(strings.TrimSpace(value))); err == nil {
		return strconv.FormatBool(parsed), nil
	}
	return "", fmt.Errorf("cannot convert %q to bool", value)
}

// parseYAMLFloat accepts YAML floats and ints (both fit an existing float).
func parseYAMLFloat(value string) (string, error) {
	tag, normalized, err := resolveScalar(value)
	if err == nil && (tag == "!!float" || tag == "!!int") {
		return normalized, nil
	}
	return "", fmt.Errorf("cannot convert %q to float", value)
}

// newScalarNode builds the node for a newly created key. Booleans and plain
// integers get their YAML type so viper reads them back as-is; everything else
// stays a string.
func newScalarNode(value string) *yaml.Node {
	if value == "true" || value == "false" {
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: value}
	}
	if decimalIntRE.MatchString(value) {
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: value}
	}
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
}

// resolveScalar parses value as a YAML scalar and returns its resolved tag.
func resolveScalar(value string) (string, string, error) {
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(value), &node); err != nil {
		return "", "", err
	}
	if len(node.Content) != 1 || node.Content[0].Kind != yaml.ScalarNode {
		return "", "", errors.New("value is not a scalar")
	}
	scalar := node.Content[0]
	return scalar.Tag, scalar.Value, nil
}

// rootMapping returns the top-level mapping of a document node, or nil when
// the document is empty or not a mapping.
func rootMapping(doc *yaml.Node) *yaml.Node {
	if doc == nil || doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil
	}
	if doc.Content[0].Kind != yaml.MappingNode {
		return nil
	}
	return doc.Content[0]
}

// ensureRootMapping returns the top-level mapping, creating it for an empty
// document (a file that exists but has no content yet).
func ensureRootMapping(doc *yaml.Node) (*yaml.Node, error) {
	if doc == nil {
		return nil, errors.New("no config document")
	}
	if doc.Kind == 0 {
		doc.Kind = yaml.DocumentNode
	}
	if doc.Kind != yaml.DocumentNode {
		return nil, errors.New("config is not a YAML document")
	}
	if len(doc.Content) == 0 {
		root := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		doc.Content = append(doc.Content, root)
		return root, nil
	}
	if doc.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("top level of the config is not a mapping")
	}
	return doc.Content[0], nil
}

// mappingValue returns the value node for key inside a mapping node.
func mappingValue(mapping *yaml.Node, key string) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

// splitPath splits a dot path and rejects empty segments ("a..b", ".a").
func splitPath(dotPath string) ([]string, error) {
	segments := strings.Split(dotPath, ".")
	for _, segment := range segments {
		if segment == "" {
			return nil, fmt.Errorf("invalid key %q", dotPath)
		}
	}
	return segments, nil
}
