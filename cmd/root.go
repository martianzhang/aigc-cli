package cmd

import (
	"fmt"
	"os"

	"github.com/martianzhang/apimart-cli/internal/config"
	"github.com/spf13/cobra"
)

var (
	cfgFile    string
	apiKey     string
	apiBase    string
	httpProxy  string
	jsonInput  string
	outputDir  string
	verbose    bool
	savePrompt bool
)

// rootCmd represents the base command.
var rootCmd = &cobra.Command{
	Use:     "apimart-cli",
	Short:   "CLI tool for APIMart image generation",
	Version: Version,
	CompletionOptions: cobra.CompletionOptions{
		HiddenDefaultCmd: true, // hide "completion" to avoid confusion with LLM completion
	},
	Long: `APIMart API 的统一命令行工具。支持图片生成、任务查询、余额查询等操作。

子命令：
  image     图片生成（文生图、图生图、Inpainting）
  video     视频生成（文生视频、图生视频）
  chat      AI 对话（流式输出，默认 deepseek-v4-flash）
  models    查询模型列表及定价（无需 API Key）
  task      查询任务状态
  balance   查询余额

默认值可在 ~/.config/apimart/config.yaml 中配置。`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Load config (optional) to resolve defaults not set via flags
		if cfg, err := config.Load(cfgFile); err == nil {
			if apiKey == "" {
				apiKey = cfg.APIKey
			}
			if apiBase == "" {
				apiBase = cfg.BaseURL
			}
			if httpProxy == "" {
				httpProxy = cfg.HTTPProxy
			}
			if !cmd.Flags().Changed("verbose") {
				verbose = cfg.Verbose
			}
			if !cmd.Flags().Changed("save-prompt") {
				savePrompt = cfg.SavePrompt
			}
		}
		if apiKey == "" {
			return fmt.Errorf("API key is required: set it via --api-key flag, APIMART_API_KEY env, or config.yaml")
		}
		return nil
	},
}

// Execute adds all child commands to the root command and sets flags.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "path to config file (default ~/.config/apimart/config.yaml)")
	rootCmd.PersistentFlags().StringVar(&apiKey, "api-key", "", "APIMart API key (env: APIMART_API_KEY)")
	rootCmd.PersistentFlags().StringVar(&apiBase, "api-base", "", "API base URL (env: APIMART_API_BASE, default https://api.apimart.ai)")
	rootCmd.PersistentFlags().StringVar(&httpProxy, "http-proxy", "", "HTTP proxy URL (env: APIMART_HTTP_PROXY, e.g. http://127.0.0.1:7890)")
	rootCmd.PersistentFlags().StringVar(&jsonInput, "json", "", "JSON file path, JSON string, or \"-\" for stdin")

	rootCmd.PersistentFlags().StringVar(&outputDir, "output", ".", "output directory for downloaded images")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output: show full result JSON")
	rootCmd.PersistentFlags().BoolVar(&savePrompt, "save-prompt", false, "save prompt to .md file alongside results")
}
