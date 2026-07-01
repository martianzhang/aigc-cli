package cmd

import "github.com/martianzhang/apimart-cli/internal/types"

// SharedConfig holds all shared configuration values that were previously
// individual global variables. Initialized in PersistentPreRunE.
type SharedConfig struct {
	CfgFile     string
	APIKey      string
	APIBase     string
	HTTPProxy   string
	Model       string
	JSONInput   string
	OutputDir   string
	Verbose     bool
	SavePrompt  bool
	Mode        string
	PrintConfig bool
	TimeoutFlag int
	Cfg         *types.Config // full parsed config (may be nil)
}
