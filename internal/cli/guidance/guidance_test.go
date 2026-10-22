package guidance

import (
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func newTestCommand() (*cobra.Command, *cobra.Command) {
	root := &cobra.Command{Use: "aigc-cli"}
	image := &cobra.Command{Use: "image"}
	image.Flags().String("model", "", "model name")
	image.Flags().Int("n", 1, "number of images")
	image.Flags().Bool("force", false, "overwrite output")
	image.Flags().Duration("timeout", 0, "timeout")
	image.Flags().StringArray("tag", nil, "tags")
	image.Flags().StringP("output", "o", ".", "output directory")
	image.Flags().StringP("mode", "M", "auto", "generation mode")
	image.Flags().IntP("width", "w", 0, "image width")
	root.AddCommand(image)
	return root, image
}

func TestExplainUnknownFlagSuggestsClosest(t *testing.T) {
	_, image := newTestCommand()
	err := image.Flags().Parse([]string{"--modle", "x"})
	if err == nil {
		t.Fatal("Parse() error = nil, want unknown flag")
	}

	lines := Explain(image, err)
	if len(lines) == 0 {
		t.Fatal("Explain() returned no hints")
	}
	if !strings.Contains(lines[0], "--model") {
		t.Errorf("first hint = %q, want a --model suggestion", lines[0])
	}
	assertHelpHint(t, lines, "aigc-cli image")
}

func TestExplainUnknownFlagTransposition(t *testing.T) {
	_, image := newTestCommand()
	err := image.Flags().Parse([]string{"--ouptut", "x"})

	lines := Explain(image, err)
	if len(lines) == 0 || !strings.Contains(lines[0], "--output") {
		t.Fatalf("Explain() = %v, want a --output suggestion", lines)
	}
}

func TestExplainUnknownFlagWithoutMatch(t *testing.T) {
	_, image := newTestCommand()
	err := image.Flags().Parse([]string{"--zzzzzzzz"})

	lines := Explain(image, err)
	if len(lines) == 0 {
		t.Fatal("Explain() returned no hints")
	}
	if !strings.Contains(lines[0], "Unknown option --zzzzzzzz") {
		t.Errorf("first hint = %q, want an unknown-option message", lines[0])
	}
}

func TestExplainUnknownShorthandListsAvailable(t *testing.T) {
	_, image := newTestCommand()
	err := image.Flags().Parse([]string{"-x"})

	lines := Explain(image, err)
	if len(lines) == 0 {
		t.Fatal("Explain() returned no hints")
	}
	if !strings.Contains(lines[0], "Unknown shorthand -x") {
		t.Errorf("first hint = %q, want shorthand message", lines[0])
	}
	for _, want := range []string{"-o (--output)", "-w (--width)"} {
		if !strings.Contains(lines[0], want) {
			t.Errorf("hint %q does not list %q", lines[0], want)
		}
	}
}

func TestExplainInvalidIntegerExample(t *testing.T) {
	_, image := newTestCommand()
	err := image.Flags().Parse([]string{"--n", "abc"})

	lines := Explain(image, err)
	if len(lines) == 0 {
		t.Fatal("Explain() returned no hints")
	}
	want := `Invalid value "abc" for --n: expected an integer, such as 1.`
	if lines[0] != want {
		t.Errorf("first hint = %q, want %q", lines[0], want)
	}
	assertHelpHint(t, lines, "aigc-cli image")
}

func TestExplainInvalidBoolExample(t *testing.T) {
	_, image := newTestCommand()
	err := image.Flags().Parse([]string{"--force=maybe"})

	lines := Explain(image, err)
	if len(lines) == 0 || !strings.Contains(lines[0], "expected true or false") {
		t.Fatalf("Explain() = %v, want bool guidance", lines)
	}
}

func TestExplainInvalidDurationExample(t *testing.T) {
	_, image := newTestCommand()
	err := image.Flags().Parse([]string{"--timeout", "soon"})

	lines := Explain(image, err)
	if len(lines) == 0 || !strings.Contains(lines[0], "expected a duration, such as 30s") {
		t.Fatalf("Explain() = %v, want duration guidance", lines)
	}
}

func TestExplainMissingValue(t *testing.T) {
	_, image := newTestCommand()
	err := image.Flags().Parse([]string{"--model"})

	lines := Explain(image, err)
	if len(lines) == 0 {
		t.Fatal("Explain() returned no hints")
	}
	want := "Option --model needs a value; pass it as --model VALUE."
	if lines[0] != want {
		t.Errorf("first hint = %q, want %q", lines[0], want)
	}
}

func TestExplainRequiredFlags(t *testing.T) {
	root, image := newTestCommand()

	lines := Explain(image, errors.New(`required flag(s) "prompt", "model" not set`))
	if len(lines) == 0 {
		t.Fatal("Explain() returned no hints")
	}
	want := "Required option(s) missing: --prompt, --model."
	if lines[0] != want {
		t.Errorf("first hint = %q, want %q", lines[0], want)
	}
	assertHelpHint(t, lines, "aigc-cli image")
	_ = root
}

func TestExplainUnknownCommand(t *testing.T) {
	root, _ := newTestCommand()

	lines := Explain(root, errors.New(`unknown command "imag" for "aigc-cli"`))
	if len(lines) != 1 {
		t.Fatalf("Explain() = %v, want one help hint", lines)
	}
	if !strings.Contains(lines[0], "aigc-cli --help") {
		t.Errorf("hint = %q, want root help pointer", lines[0])
	}
}

func TestExplainRuntimeErrorHasNoHint(t *testing.T) {
	_, image := newTestCommand()

	if lines := Explain(image, errors.New("API request failed")); len(lines) != 0 {
		t.Errorf("Explain() = %v, want no hints for runtime errors", lines)
	}
}

func TestExplainHiddenFlagsAreNotSuggested(t *testing.T) {
	_, image := newTestCommand()
	if err := image.Flags().MarkHidden("force"); err != nil {
		t.Fatal(err)
	}
	err := image.Flags().Parse([]string{"--forc"})

	lines := Explain(image, err)
	if len(lines) == 0 {
		t.Fatal("Explain() returned no hints")
	}
	if strings.Contains(lines[0], "--force") {
		t.Errorf("hint %q suggests a hidden flag", lines[0])
	}
}

func TestEditDistance(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"model", "model", 0},
		{"modle", "model", 1},
		{"mod", "model", 2},
		{"output", "ouptut", 1},
		{"", "model", 5},
	}
	for _, tt := range tests {
		if got := editDistance(tt.a, tt.b); got != tt.want {
			t.Errorf("editDistance(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func assertHelpHint(t *testing.T, lines []string, path string) {
	t.Helper()
	want := "Run '" + path + " --help' for usage and examples."
	for _, line := range lines {
		if line == want {
			return
		}
	}
	t.Errorf("hints = %v, want %q", lines, want)
}
