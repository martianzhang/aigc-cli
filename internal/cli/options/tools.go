package options

import "path"

func matchAny(name string, patterns []string) bool {
	for _, p := range patterns {
		if matched, _ := path.Match(p, name); matched {
			return true
		}
	}
	return false
}

// IsToolAllowed checks if a tool is allowed by global tools_enable/tools_disable
// rules. An empty enable list allows all tools; disable is a blacklist on top.
func IsToolAllowed(toolName string, enable, disable []string) bool {
	if len(enable) > 0 && !matchAny(toolName, enable) {
		return false
	}
	if matchAny(toolName, disable) {
		return false
	}
	return true
}
