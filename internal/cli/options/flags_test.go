package options

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestSetIntFlag_changed(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Int("test-flag", 0, "")
	cmd.Flags().Set("test-flag", "42")

	var target *int
	SetIntFlag(cmd, "test-flag", &target, 42)
	if target == nil || *target != 42 {
		t.Error("SetIntFlag should set target when flag is changed")
	}
}

func TestSetIntFlag_notChanged(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Int("test-flag", 0, "")

	var target *int
	SetIntFlag(cmd, "test-flag", &target, 42)
	if target != nil {
		t.Error("SetIntFlag should not set target when flag is not changed")
	}
}

func TestSetBoolFlag_changed(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("test-flag", false, "")
	cmd.Flags().Set("test-flag", "true")

	var target *bool
	SetBoolFlag(cmd, "test-flag", &target, true)
	if target == nil || *target != true {
		t.Error("SetBoolFlag should set target when flag is changed")
	}
}

func TestSetBoolFlag_notChanged(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("test-flag", false, "")

	var target *bool
	SetBoolFlag(cmd, "test-flag", &target, true)
	if target != nil {
		t.Error("SetBoolFlag should not set target when flag is not changed")
	}
}

func TestSetFloatFlag_changed(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Float64("test-flag", 0, "")
	cmd.Flags().Set("test-flag", "0.7")

	var target *float64
	SetFloatFlag(cmd, "test-flag", &target, 0.7)
	if target == nil || *target != 0.7 {
		t.Error("SetFloatFlag should set target when flag is changed")
	}
}

func TestSetFloatFlag_notChanged(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Float64("test-flag", 0, "")

	var target *float64
	SetFloatFlag(cmd, "test-flag", &target, 0.7)
	if target != nil {
		t.Error("SetFloatFlag should not set target when flag is not changed")
	}
}
