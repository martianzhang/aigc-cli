package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/martianzhang/aigc-cli/internal/config"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// newConfigCmd builds the `config` command tree.
func newConfigCmd() *cobra.Command {
	configCmd := &cobra.Command{
		Use:          "config",
		Short:        "Read or edit the config file (get/set/list)",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		Long: `Read and edit the YAML config file without opening an editor.

Keys are dot paths matching the YAML structure: defaults.image.model,
providers.openrouter.api_key, api_key.

The file is resolved like everywhere else: --config <path> first, otherwise
~/.config/aigc-cli/config.yaml. get/set never create the file or missing
sections. set keeps the existing YAML type of a key, backs the file up to
<path>.bak and replaces it atomically, so comments, key order and formatting
survive.`,
		Example: `  aigc-cli config get defaults.image.model
  aigc-cli config set defaults.image.model gpt-image-2
  aigc-cli config set api_key sk-xxx --force
  aigc-cli config list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	getCmd := &cobra.Command{
		Use:          "get <key>",
		Short:        "Print one config value (secrets masked)",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE:         runConfigGet,
	}

	setCmd := &cobra.Command{
		Use:          "set <key> <value>",
		Short:        "Set one config value (atomic write with backup)",
		Args:         cobra.ExactArgs(2),
		SilenceUsage: true,
		RunE:         runConfigSet,
	}
	setCmd.Flags().Bool("force", false, "confirm writing a key that holds credentials or an endpoint (api_key/base_url)")

	listCmd := &cobra.Command{
		Use:          "list",
		Short:        "Print the effective config with secrets masked",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE:         runConfigList,
	}

	configCmd.AddCommand(getCmd, setCmd, listCmd)
	return configCmd
}

func init() {
	rootCmd.AddCommand(newConfigCmd())
}

// runConfigGet prints the value at a dot path from the config file.
func runConfigGet(cmd *cobra.Command, args []string) error {
	key := args[0]
	path, err := configFilePath()
	if err != nil {
		return err
	}
	doc, err := loadExistingConfig(path)
	if err != nil {
		return err
	}
	node, err := config.NodeAt(doc, key)
	if err != nil {
		return fmt.Errorf("get %s: %w", key, err)
	}
	return printConfigNode(cmd.OutOrStdout(), key, node)
}

// runConfigSet writes one scalar back to the config file.
func runConfigSet(cmd *cobra.Command, args []string) error {
	key, value := args[0], args[1]
	force, err := cmd.Flags().GetBool("force")
	if err != nil {
		return err
	}
	if isSensitiveConfigKey(key) && !force {
		return fmt.Errorf("refusing to set %s without --force: api_key/base_url hold credentials or endpoint overrides", key)
	}
	path, err := configFilePath()
	if err != nil {
		return err
	}
	doc, err := loadExistingConfig(path)
	if err != nil {
		return err
	}
	if err := config.SetScalar(doc, key, value); err != nil {
		return fmt.Errorf("set %s: %w", key, err)
	}
	if err := config.SaveNode(path, doc); err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "set %s = %s\n", key, maskConfigLeaf(key, value))
	fmt.Fprintf(out, "backup: %s.bak\n", path)
	return nil
}

// runConfigList prints the effective config (file + env + defaults) with the
// same masking as --print-config.
func runConfigList(cmd *cobra.Command, args []string) error {
	path, err := configFilePath()
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	cfg := &types.Config{BaseURL: types.DefaultAPIBaseURL}
	if _, statErr := os.Stat(path); statErr == nil {
		loaded, err := config.Load(path)
		if err != nil {
			return err
		}
		cfg = loaded
	} else if !os.IsNotExist(statErr) {
		return fmt.Errorf("config file: %w", statErr)
	} else {
		fmt.Fprintf(out, "# config file not found: %s (showing code defaults)\n", path)
	}

	masked := service.MaskConfigSecrets(cfg)
	if masked == nil {
		masked = cfg
	}
	data, err := yaml.Marshal(configDisplay{Config: *masked})
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	_, err = out.Write(data)
	return err
}
