package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/knowledge"
	"github.com/spf13/cobra"
)

// kbBaseDir is the knowledge base data directory.
var kbBaseDir string

// vaultBaseDir is the vault data directory.
var vaultBaseDir string

// vaultEnabled is true when --vault flag is set.
var vaultEnabled bool

// kbCmd represents the `kb` parent command.
var kbCmd = &cobra.Command{
	Use:     "knowledgebase",
	Aliases: []string{"kb"},
	Short:   "Local knowledge base management (also: kb)",
	Long: `A local knowledge base with full-text and semantic search (also: kb).

  knowledgebase init          Initialize the knowledge base
  knowledgebase add <path>    Add a local file or directory
  knowledgebase fetch <url>   Fetch a URL and add it
  knowledgebase map <url>     Discover URls from a page and batch fetch
  knowledgebase find <query>  Search the knowledge base
  knowledgebase search <q>    Web search + save to knowledge base
  knowledgebase list          List all documents
  knowledgebase show <id>     Show document details
  knowledgebase rm <id>       Remove a document
  knowledgebase prune          Remove duplicates and dead URLs
  knowledgebase reset          Delete all data and reinitialize
  knowledgebase index         Re-index all documents

Use --project to scope documents to the current git repository.
Use --all to search across all projects (for find/list).

Data stored at ~/.config/aigc-cli/knowledge/ by default.`,
	SilenceUsage: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Override kbBaseDir from config if flag wasn't explicitly set
		if !cmd.Flags().Changed("dir") {
			if d := resolveConfigBaseDir(); d != "" {
				kbBaseDir = d
			}
		}
		return nil
	},
}

func init() {
	kbDir := filepath.Join(configDir(), "knowledge")
	vaultDir := filepath.Join(configDir(), "vault")
	vaultBaseDir = vaultDir
	kbCmd.PersistentFlags().StringVar(&kbBaseDir, "dir", kbDir, "Knowledge base data directory")
	kbCmd.PersistentFlags().BoolVar(&vaultEnabled, "vault", false, "Operate on the vault (encrypted storage)")
	kbCmd.PersistentFlags().Bool("project", false, "Scope to current git repository")
	kbCmd.PersistentFlags().Bool("all", false, "Search across all projects (default)")

	kbCmd.AddCommand(kbInitCmd)
	kbCmd.AddCommand(kbAddCmd)
	kbCmd.AddCommand(kbFetchCmd)
	kbCmd.AddCommand(kbMapCmd)
	kbCmd.AddCommand(kbFindCmd)
	kbCmd.AddCommand(kbSearchCmd)
	kbCmd.AddCommand(kbListCmd)
	kbCmd.AddCommand(kbShowCmd)
	kbCmd.AddCommand(kbRmCmd)
	kbCmd.AddCommand(kbPruneCmd)
	kbCmd.AddCommand(kbResetCmd)
	kbCmd.AddCommand(kbIndexCmd)
	kbCmd.AddCommand(kbVaultCmd)
	rootCmd.AddCommand(kbCmd)
}

// ensureKBDir creates the knowledge base directory if it doesn't exist.
func ensureKBDir() error {
	if err := os.MkdirAll(kbBaseDir, 0755); err != nil {
		return fmt.Errorf("create kb dir: %w", err)
	}
	return nil
}

// onnxLibPath returns the path to the ONNX Runtime shared library.
func onnxLibPath() string {
	var name string
	switch runtime.GOOS {
	case "darwin":
		name = "libonnxruntime.dylib"
	case "linux":
		name = "libonnxruntime.so"
	default:
		name = "onnxruntime.dll"
	}
	modelsDir := filepath.Join(configDir(), "models")
	path := filepath.Join(modelsDir, name)
	if _, err := os.Stat(path); err == nil {
		return path
	}
	return ""
}

// openKBStore opens the knowledge base store with the best available embedder.
func openKBStore() (*knowledge.Store, error) {
	if err := ensureKBDir(); err != nil {
		return nil, err
	}
	modelsDir := filepath.Join(configDir(), "models")
	libPath := onnxLibPath()
	embedder := knowledge.BestEmbedder(modelsDir, libPath)
	store, err := knowledge.OpenStore(kbBaseDir, 384, embedder)
	if err != nil {
		return nil, err
	}
	// Set min score threshold from config
	if shared.Cfg != nil && shared.Cfg.Defaults != nil && shared.Cfg.Defaults.Knowledgebase != nil {
		if ms := shared.Cfg.Defaults.Knowledgebase.MinScore; ms > 0 {
			store.SetMinScore(ms)
		}
	}
	return store, nil
}

// resolveConfigBaseDir returns the base_dir from config defaults if set.
func resolveConfigBaseDir() string {
	if shared.Cfg != nil && shared.Cfg.Defaults != nil && shared.Cfg.Defaults.Knowledgebase != nil {
		return shared.Cfg.Defaults.Knowledgebase.BaseDir
	}
	return ""
}

// shouldAutoSave returns whether kb search results should be saved to the KB.
// Priority: CLI --auto-save flag > config defaults.auto_save > default (true).
func shouldAutoSave() bool {
	// CLI --save flag (default true, only applies when explicitly set)
	if !kbSearchSaveFlag {
		return false
	}
	// Config auto_save (only when explicitly configured)
	if shared.Cfg != nil && shared.Cfg.Defaults != nil && shared.Cfg.Defaults.Knowledgebase != nil {
		if shared.Cfg.Defaults.Knowledgebase.AutoSave != nil {
			return *shared.Cfg.Defaults.Knowledgebase.AutoSave
		}
	}
	return true
}

// resolveLoaders returns the external file loaders from config.
func resolveLoaders() map[string]string {
	if shared.Cfg != nil && shared.Cfg.Defaults != nil && shared.Cfg.Defaults.Knowledgebase != nil {
		return shared.Cfg.Defaults.Knowledgebase.Loaders
	}
	return nil
}

// resolveSearchProject returns the project scope for find/search commands.
// --all → "" (all projects), --project or default → specific project, else "".
func resolveSearchProject(cmd *cobra.Command) string {
	all, _ := cmd.Flags().GetBool("all")
	if all {
		return ""
	}
	return detectProject(cmd)
}

// detectProject returns a unique project identifier when --project is implied or set.
// Uses git remote origin URL (e.g., "github.com/martianzhang/aigc-cli") for uniqueness.
// Falls back to git root basename.
// Returns "" for global scope.
func detectProject(cmd *cobra.Command) string {
	project, _ := cmd.Flags().GetBool("project")
	if !project {
		all, _ := cmd.Flags().GetBool("all")
		if all {
			return ""
		}
		project = true // auto-detect when in a git repo
	}
	if !project {
		return ""
	}

	// Try git remote origin URL first
	if id, err := gitProjectID(); err == nil && id != "" {
		return id
	}
	return ""
}

// gitProjectID returns a unique project ID from the git remote origin URL.
// "git@github.com:martianzhang/aigc-cli.git" → "github.com/martianzhang/aigc-cli"
// "https://github.com/org/repo.git" → "github.com/org/repo"
func gitProjectID() (string, error) {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	raw := strings.TrimSpace(string(out))
	raw = strings.TrimSuffix(raw, ".git")
	for _, prefix := range []string{"https://", "http://"} {
		raw = strings.TrimPrefix(raw, prefix)
	}
	if strings.HasPrefix(raw, "git@") {
		raw = strings.Replace(raw, ":", "/", 1)
		raw = strings.TrimPrefix(raw, "git@")
	}
	return raw, nil
}

// isGlobalScope returns true when listing/finding across all projects.
func isGlobalScope(cmd *cobra.Command) bool {
	all, _ := cmd.Flags().GetBool("all")
	return all
}
