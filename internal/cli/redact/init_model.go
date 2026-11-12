package redact

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/modelstore"
	"github.com/martianzhang/aigc-cli/internal/options"
	redactlib "github.com/martianzhang/aigc-cli/internal/redact"
	"github.com/martianzhang/aigc-cli/internal/terminal"
)

func writeTokenclsModelsTable(out io.Writer) error {
	headers := []string{"ID", "NAME", "SIZE", "LICENSE", "SOURCE"}
	rows := make([][]string, 0)
	for _, model := range redactlib.TokenclsModels() {
		var size int64
		for _, file := range model.Files {
			size += file.SizeMB
		}
		rows = append(rows, []string{
			model.ID, model.Name, fmt.Sprintf("%d MB", size), model.License, model.Source,
		})
	}
	if err := terminal.WriteTable(out, headers, rows, terminal.TableOptions{}); err != nil {
		return err
	}
	fmt.Fprintln(out, "\nInstall with: aigc-cli redact init --model <id>")
	return nil
}

func installTokenclsModel(cmd *cobra.Command, id, modelsRoot string, force bool) error {
	info, ok := redactlib.FindTokenclsModel(id)
	if !ok {
		ids := make([]string, 0)
		for _, model := range redactlib.TokenclsModels() {
			ids = append(ids, model.ID)
		}
		return fmt.Errorf("unknown tokencls model %q (available: %s)", id, strings.Join(ids, ", "))
	}
	return installTokenclsModelInfo(cmd.OutOrStdout(), cmd.ErrOrStderr(), info, modelsRoot, force)
}

func installTokenclsModelInfo(out, progress io.Writer, info redactlib.TokenclsModelInfo, modelsRoot string, force bool) error {
	root := strings.TrimSpace(modelsRoot)
	if root == "" {
		root = defaultModelsRoot()
	}
	dir := filepath.Join(expandHome(root), "redact", info.ID)

	fmt.Fprintf(progress, "Installing %s (%s) into %s\n", info.Name, info.License, dir)
	if err := downloadTokenclsFiles(progress, dir, info, force); err != nil {
		return err
	}
	fmt.Fprintf(out, "\nInstalled %s.\n\nConfigure:\n\ndefaults:\n  redact:\n    model_engine: tokencls\n    tokencls:\n      model: %s\n\nSource:   %s\nLicense:  %s (see %s)\n",
		info.ID, info.ID, info.Source, info.License, filepath.Join(dir, "LICENSE.txt"))
	return nil
}

// downloadTokenclsFiles installs every file in info into dir through the shared
// modelstore (checksum + Range resume + TOFU allowlist for empty digests).
func downloadTokenclsFiles(out io.Writer, dir string, info redactlib.TokenclsModelInfo, force bool) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create model directory: %w", err)
	}
	for _, file := range info.Files {
		dest := filepath.Join(dir, file.OutName)
		fmt.Fprintf(out, "  Downloading %s (%d MB)...\n", file.OutName, file.SizeMB)
		if err := modelstore.Install(options.Context(), nil, file.URL, dest, modelstore.Options{Force: force, SHA256: file.SHA256}); err != nil {
			return fmt.Errorf("install %s: %w", file.OutName, err)
		}
		fmt.Fprintf(out, "  ✓ %s\n", file.OutName)
	}
	return nil
}

func defaultModelsRoot() string {
	if d.ResolveProvider != nil {
		if p := d.ResolveProvider(options.ProviderNameRedact); p != nil && p.ModelsDir != "" {
			return p.ModelsDir
		}
	}
	return filepath.Join(options.ConfigDir(), "models")
}

func expandHome(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	return path
}
