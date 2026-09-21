package service

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeJSONC(t *testing.T) {
	tests := []struct {
		name      string
		in        string
		want      string
		parseable bool
	}{
		{
			name:      "line comments leading trailing and between members",
			in:        "{\n  // leading\n  \"a\": 1, // trailing\n  // between\n  \"b\": 2\n}",
			want:      "{\n  \n  \"a\": 1, \n  \n  \"b\": 2\n}",
			parseable: true,
		},
		{
			name:      "line comment before array close with trailing comma",
			in:        "[1, 2, // c\n]",
			want:      "[1, 2 \n]",
			parseable: true,
		},
		{
			name:      "single line block comment",
			in:        `{"a": /* one */ 1}`,
			want:      `{"a":  1}`,
			parseable: true,
		},
		{
			name:      "multi line block comment",
			in:        "{\"a\": /* multi\nline */ 1}",
			want:      "{\"a\":  1}",
			parseable: true,
		},
		{
			name:      "comment markers inside strings preserved",
			in:        `{"u":"https://host//path","v":"a/*b*/c"}`,
			want:      `{"u":"https://host//path","v":"a/*b*/c"}`,
			parseable: true,
		},
		{
			name:      "escaped quote does not end string",
			in:        `{"s":"a \" // not a comment"}`,
			want:      `{"s":"a \" // not a comment"}`,
			parseable: true,
		},
		{
			name:      "escaped backslash before closing quote",
			in:        `{"s":"a\\"}`,
			want:      `{"s":"a\\"}`,
			parseable: true,
		},
		{
			name:      "trailing comma in object",
			in:        `{"a": 1,}`,
			want:      `{"a": 1}`,
			parseable: true,
		},
		{
			name:      "trailing comma in nested array and object",
			in:        `{"a": [1, 2, ], "b": {"c": 3, }, }`,
			want:      `{"a": [1, 2 ], "b": {"c": 3 } }`,
			parseable: true,
		},
		{
			name:      "trailing comma followed by line comment",
			in:        "{\"a\": 1, // c\n}",
			want:      "{\"a\": 1 \n}",
			parseable: true,
		},
		{
			name:      "trailing comma followed by block comment",
			in:        `{"a": 1, /* c */}`,
			want:      `{"a": 1 }`,
			parseable: true,
		},
		{
			name:      "leading BOM dropped",
			in:        "\xEF\xBB\xBF{\"a\":1}",
			want:      `{"a":1}`,
			parseable: true,
		},
		{
			name:      "unterminated block comment drops remainder",
			in:        `{"a":1} /* never closed`,
			want:      `{"a":1} `,
			parseable: false,
		},
		{
			name:      "empty input",
			in:        "",
			want:      "",
			parseable: false,
		},
		{
			name:      "comment only input",
			in:        "// nothing\n",
			want:      "\n",
			parseable: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := NormalizeJSONC([]byte(tc.in))
			if !bytes.Equal(got, []byte(tc.want)) {
				t.Fatalf("NormalizeJSONC(%q) = %q, want %q", tc.in, got, tc.want)
			}
			if tc.parseable {
				var v any
				if err := json.Unmarshal(got, &v); err != nil {
					t.Fatalf("normalized output does not parse: %v (output %q)", err, got)
				}
			}
		})
	}
}

func TestNormalizeJSONCStrictJSONUnchanged(t *testing.T) {
	strict := []string{
		`{"a":1}`,
		`{}`,
		`[]`,
		`{"a": 1, "b": [2, 3], "c": {"d": null}}`,
		`[1,2,3]`,
		`"plain string"`,
		`{"u":"https://host//path","v":"a/*b*/c"}`,
		`{"s":"a \" // not a comment"}`,
		`{"esc":"line\nbreak\tand\\slash"}`,
		"{\n  \"prompt\": \"a cat\",\n  \"n\": 2\n}\n",
	}
	for _, in := range strict {
		var v any
		if err := json.Unmarshal([]byte(in), &v); err != nil {
			t.Fatalf("test sample is not strict JSON: %q: %v", in, err)
		}
		got := NormalizeJSONC([]byte(in))
		if !bytes.Equal(got, []byte(in)) {
			t.Errorf("NormalizeJSONC(%q) = %q, want byte-identical input", in, got)
		}
	}
}

func TestNormalizeJSONCUnterminatedBlockComment(t *testing.T) {
	inputs := []string{
		`{"a": /* never closes`,
		`{"a":1} /*`,
		"{\"a\": 1} /* multi\nline",
		`{"a":1} /*/`,
	}
	for _, in := range inputs {
		got := NormalizeJSONC([]byte(in))
		if !bytes.HasPrefix([]byte(in), got) {
			t.Errorf("NormalizeJSONC(%q) = %q, want a prefix of the input", in, got)
		}
		if len(got) > len(in) {
			t.Errorf("NormalizeJSONC(%q) = %q, want at most %d bytes", in, got, len(in))
		}
	}
}

func TestReadJSONInputNormalizesJSONC(t *testing.T) {
	t.Run("jsonc file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "body.jsonc")
		content := "{\n  // provider hint\n  \"prompt\": \"a cat\", // trailing\n  \"n\": 2,\n}\n"
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write temp jsonc: %v", err)
		}
		got, err := ReadJSONInput(path)
		if err != nil {
			t.Fatalf("ReadJSONInput() error = %v", err)
		}
		var m map[string]any
		if err := json.Unmarshal(got, &m); err != nil {
			t.Fatalf("ReadJSONInput() output does not parse: %v (output %q)", err, got)
		}
		if m["prompt"] != "a cat" || m["n"] != float64(2) {
			t.Fatalf("ReadJSONInput() = %v, want prompt=a cat and n=2", m)
		}
	})

	t.Run("inline jsonc", func(t *testing.T) {
		got, err := ReadJSONInput(`{"a": 1, /* c */}`)
		if err != nil {
			t.Fatalf("ReadJSONInput() error = %v", err)
		}
		if string(got) != `{"a": 1 }` {
			t.Fatalf("ReadJSONInput() = %q, want %q", string(got), `{"a": 1 }`)
		}
	})
}
