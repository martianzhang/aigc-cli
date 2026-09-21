package service

// NormalizeJSONC converts a JSONC document to strict JSON: it drops a leading
// UTF-8 BOM, removes // line comments and /* */ block comments, and removes
// trailing commas before } or ]. Comment markers inside string literals are
// preserved, so a value like "https://host//path" is untouched. A strict JSON
// document is returned byte-for-byte unchanged.
func NormalizeJSONC(data []byte) []byte {
	data = trimBOM(data)
	out := make([]byte, 0, len(data))
	inString := false
	for i := 0; i < len(data); {
		c := data[i]
		if inString {
			out = append(out, c)
			switch c {
			case '\\':
				if i+1 < len(data) {
					out = append(out, data[i+1])
					i += 2
					continue
				}
			case '"':
				inString = false
			}
			i++
			continue
		}
		switch {
		case c == '"':
			inString = true
			out = append(out, c)
			i++
		case c == '/' && i+1 < len(data) && data[i+1] == '/':
			i = skipLineComment(data, i+2)
		case c == '/' && i+1 < len(data) && data[i+1] == '*':
			end, ok := skipBlockComment(data, i)
			if !ok {
				return out
			}
			i = end
		case c == ',':
			if isTrailingComma(data, i+1) {
				i++
				continue
			}
			out = append(out, c)
			i++
		default:
			out = append(out, c)
			i++
		}
	}
	return out
}

func trimBOM(data []byte) []byte {
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		return data[3:]
	}
	return data
}

func skipLineComment(data []byte, i int) int {
	for i < len(data) && data[i] != '\n' {
		i++
	}
	return i
}

func skipBlockComment(data []byte, i int) (int, bool) {
	j := i + 2
	for j+1 < len(data) && (data[j] != '*' || data[j+1] != '/') {
		j++
	}
	if j+1 >= len(data) {
		return len(data), false
	}
	return j + 2, true
}

func isTrailingComma(data []byte, i int) bool {
	for i < len(data) {
		switch {
		case data[i] == ' ' || data[i] == '\t' || data[i] == '\r' || data[i] == '\n':
			i++
		case data[i] == '/' && i+1 < len(data) && data[i+1] == '/':
			i = skipLineComment(data, i+2)
		case data[i] == '/' && i+1 < len(data) && data[i+1] == '*':
			end, ok := skipBlockComment(data, i)
			if !ok {
				return true
			}
			i = end
		case data[i] == '}' || data[i] == ']':
			return true
		default:
			return false
		}
	}
	return false
}
