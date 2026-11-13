// Package hint builds actionable errors for the CLI. Every hint is a command
// or environment variable the user (or an agent) can copy directly.
package hint

import (
	"errors"
	"fmt"

	"github.com/martianzhang/aigc-cli/internal/provider"
)

// Error returns "msg\nHint: hint" so cobra's "Error:" prefix yields the
// two-line format agents expect. An empty hint returns a plain error.
func Error(msg, hint string) error {
	if hint == "" {
		return errors.New(msg)
	}
	return fmt.Errorf("%s\nHint: %s", msg, hint)
}

// Wrap keeps the original error in the chain and appends a hint.
func Wrap(err error, hint string) error {
	if err == nil {
		return nil
	}
	if hint == "" {
		return err
	}
	return fmt.Errorf("%w\nHint: %s", err, hint)
}

// ProviderName returns the name an agent should pass to --provider. Only a
// configured provider name is valid; a detected protocol type such as "openai"
// is not necessarily a key under config.providers.
func ProviderName(p *provider.EffectiveProvider) string {
	if p == nil || p.Name == "" {
		return "<name>"
	}
	return p.Name
}

// MissingModel is the shared "no model configured" error for image/video.
func MissingModel(mediaType string, p *provider.EffectiveProvider) error {
	return Error("model is required", fmt.Sprintf(
		"run '%s', then pass -m <model> (or set defaults.%s.model; list providers with 'aigc-cli models --list-providers')",
		ModelDiscover(mediaType, p), mediaType))
}

// MissingLocalModel is the shared "model file not downloaded" error.
func MissingLocalModel(command, modelID, path string) error {
	return Error(fmt.Sprintf("model %q not found at %s", modelID, path), fmt.Sprintf(
		"run 'aigc-cli %s init --model %s'", command, modelID))
}

// ModelDiscover returns the command that lists models for a media type using
// the resolved provider. Listing models without a provider is ambiguous
// because every vendor publishes a different catalog.
func ModelDiscover(mediaType string, p *provider.EffectiveProvider) string {
	return fmt.Sprintf("aigc-cli models --type %s --provider %s", mediaType, ProviderName(p))
}
