package options

import "github.com/spf13/cobra"

// HasFlagChanged reports whether a flag was changed on cmd, its persistent
// flags, or its inherited flags.
func HasFlagChanged(cmd *cobra.Command, name string) bool {
	return cmd.Flags().Changed(name) || cmd.PersistentFlags().Changed(name) || cmd.InheritedFlags().Changed(name)
}

// SetIntFlag sets a *int field from a cobra flag if it was changed.
func SetIntFlag(cmd *cobra.Command, name string, target **int, val int) {
	if cmd.Flags().Changed(name) {
		v := val
		*target = &v
	}
}

// SetBoolFlag sets a *bool field from a cobra flag if it was changed.
func SetBoolFlag(cmd *cobra.Command, name string, target **bool, val bool) {
	if cmd.Flags().Changed(name) {
		v := val
		*target = &v
	}
}

// SetFloatFlag sets a *float64 field from a cobra flag if it was changed.
func SetFloatFlag(cmd *cobra.Command, name string, target **float64, val float64) {
	if cmd.Flags().Changed(name) {
		v := val
		*target = &v
	}
}
