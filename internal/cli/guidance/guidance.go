// Package guidance turns cobra/pflag usage errors into actionable hints:
// flag suggestions, type examples, and a command-scoped help pointer.
package guidance

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Explain returns extra lines to print after err. It returns nil for runtime
// errors (API failures, timeouts) so only usage errors gain guidance.
func Explain(cmd *cobra.Command, err error) []string {
	if err == nil {
		return nil
	}
	if strings.HasPrefix(err.Error(), "unknown command") {
		return []string{helpHint(cmd)}
	}

	var lines []string

	var notExist *pflag.NotExistError
	var invalidValue *pflag.InvalidValueError
	var missingValue *pflag.ValueRequiredError
	var badSyntax *pflag.InvalidSyntaxError

	switch {
	case errors.As(err, &notExist):
		lines = append(lines, notExistHint(cmd, notExist))
	case errors.As(err, &invalidValue):
		flag := invalidValue.GetFlag()
		lines = append(lines, fmt.Sprintf("Invalid value %q for --%s: %s.",
			invalidValue.GetValue(), flag.Name, valueExample(flag)))
	case errors.As(err, &missingValue):
		flag := missingValue.GetFlag()
		lines = append(lines, fmt.Sprintf("Option --%s needs a value; pass it as --%s VALUE.",
			flag.Name, flag.Name))
	case errors.As(err, &badSyntax):
		lines = append(lines, fmt.Sprintf("Bad option syntax %q; options look like --name value.",
			badSyntax.GetSpecifiedFlag()))
	default:
		if hint, ok := requiredFlagHint(err.Error()); ok {
			lines = append(lines, hint)
		}
	}

	if len(lines) == 0 {
		return nil
	}
	return append(lines, helpHint(cmd))
}

// notExistHint explains an unknown long flag or shorthand. When the name is
// close to a real flag it suggests the closest matches; otherwise it points at
// the command's own help instead of guessing.
func notExistHint(cmd *cobra.Command, err *pflag.NotExistError) string {
	name := err.GetSpecifiedName()
	if err.GetSpecifiedShortnames() != "" {
		available := shorthandList(cmd)
		if available == "" {
			return fmt.Sprintf("Unknown shorthand -%s; this command has no shorthand options.", name)
		}
		return fmt.Sprintf("Unknown shorthand -%s. Available shorthands: %s.", name, available)
	}

	suggestions := suggestFlags(cmd, name)
	if len(suggestions) == 0 {
		return fmt.Sprintf("Unknown option --%s; the help command below lists every option.", name)
	}

	for i, suggestion := range suggestions {
		suggestions[i] = "--" + suggestion
	}
	return "Did you mean " + strings.Join(suggestions, ", ") + "?"
}

// suggestFlags returns up to three flag names close to the misspelled input.
// Prefix matches rank first; otherwise a Damerau-Levenshtein distance (which
// treats transpositions as one edit) decides.
func suggestFlags(cmd *cobra.Command, input string) []string {
	if cmd == nil || len(input) < 2 {
		return nil
	}
	lowered := strings.ToLower(input)

	type candidate struct {
		name     string
		distance int
	}
	var candidates []candidate

	visitFlags(cmd, func(flag *pflag.Flag) {
		if flag.Hidden {
			return
		}
		flagName := strings.ToLower(flag.Name)
		distance := editDistance(lowered, flagName)
		if strings.HasPrefix(flagName, lowered) || strings.HasPrefix(lowered, flagName) {
			distance = 0
		}
		limit := 2
		if len(flagName) >= 8 {
			limit = 3
		}
		if distance <= limit {
			candidates = append(candidates, candidate{name: flag.Name, distance: distance})
		}
	})

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].distance != candidates[j].distance {
			return candidates[i].distance < candidates[j].distance
		}
		return candidates[i].name < candidates[j].name
	})
	if len(candidates) > 3 {
		candidates = candidates[:3]
	}

	names := make([]string, len(candidates))
	for i, candidate := range candidates {
		names[i] = candidate.name
	}
	return names
}

// shorthandList summarizes the shorthands a command actually defines.
func shorthandList(cmd *cobra.Command) string {
	var parts []string
	visitFlags(cmd, func(flag *pflag.Flag) {
		if flag.Hidden || flag.Shorthand == "" {
			return
		}
		parts = append(parts, fmt.Sprintf("-%s (--%s)", flag.Shorthand, flag.Name))
	})
	sort.Strings(parts)
	truncated := false
	if len(parts) > 6 {
		parts = parts[:6]
		truncated = true
	}
	joined := strings.Join(parts, ", ")
	if truncated {
		return joined + ", and more"
	}
	return joined
}

// valueExample describes the value a flag expects, using a concrete sample for
// standard types.
func valueExample(flag *pflag.Flag) string {
	switch strings.ToLower(flag.Value.Type()) {
	case "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64", "count":
		return "expected an integer, such as 1"
	case "float32", "float64":
		return "expected a number, such as 0.5"
	case "bool":
		return "expected true or false"
	case "duration":
		return "expected a duration, such as 30s"
	case "stringarray", "stringslice":
		return "expected a value; repeat the option for multiple values"
	case "string":
		return "expected a text value"
	default:
		return "expected a value accepted by this option"
	}
}

// requiredFlagHint recognizes cobra's required-flag error and lists the flags
// with dashes so users can copy them.
func requiredFlagHint(message string) (string, bool) {
	prefix := "required flag(s) "
	if !strings.HasPrefix(message, prefix) || !strings.HasSuffix(message, " not set") {
		return "", false
	}
	raw := strings.TrimSuffix(strings.TrimPrefix(message, prefix), " not set")
	names := strings.Split(strings.ReplaceAll(raw, `"`, ""), ", ")
	for i, name := range names {
		names[i] = "--" + strings.TrimSpace(name)
	}
	return "Required option(s) missing: " + strings.Join(names, ", ") + ".", true
}

// helpHint points at the failing command, not the root, so users see the
// options that actually belong to the invocation.
func helpHint(cmd *cobra.Command) string {
	if cmd == nil {
		return "Run 'aigc-cli --help' for usage."
	}
	path := cmd.CommandPath()
	if path == "" {
		path = "aigc-cli"
	}
	if path == "aigc-cli" {
		return "Run 'aigc-cli --help' for commands and global options."
	}
	return fmt.Sprintf("Run '%s --help' for usage and examples.", path)
}

// visitFlags walks every flag visible on cmd, including inherited persistent
// flags, without reporting duplicates.
func visitFlags(cmd *cobra.Command, visit func(*pflag.Flag)) {
	seen := make(map[string]bool)
	cmd.Flags().VisitAll(func(flag *pflag.Flag) {
		if seen[flag.Name] {
			return
		}
		seen[flag.Name] = true
		visit(flag)
	})
}

// editDistance is the optimal string alignment distance: Levenshtein plus
// adjacent transpositions, so "modle" is one edit from "model".
func editDistance(a, b string) int {
	ar, br := []rune(a), []rune(b)
	rows := len(ar) + 1
	cols := len(br) + 1
	matrix := make([][]int, rows)
	for i := range matrix {
		matrix[i] = make([]int, cols)
		matrix[i][0] = i
	}
	for j := 0; j < cols; j++ {
		matrix[0][j] = j
	}

	for i := 1; i < rows; i++ {
		for j := 1; j < cols; j++ {
			cost := 1
			if ar[i-1] == br[j-1] {
				cost = 0
			}
			matrix[i][j] = min(
				matrix[i-1][j]+1,
				matrix[i][j-1]+1,
				matrix[i-1][j-1]+cost,
			)
			if i > 1 && j > 1 && ar[i-1] == br[j-2] && ar[i-2] == br[j-1] {
				matrix[i][j] = min(matrix[i][j], matrix[i-2][j-2]+1)
			}
		}
	}
	return matrix[rows-1][cols-1]
}
