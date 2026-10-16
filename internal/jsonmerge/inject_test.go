package jsonmerge

import (
	"encoding/json"
	"testing"
)

func TestInject(t *testing.T) {
	tests := []struct {
		name   string
		base   string
		inject string
		want   string
	}{
		{
			name:   "empty inject returns base unchanged",
			base:   `{"a":1}`,
			inject: ``,
			want:   `{"a":1}`,
		},
		{
			name:   "scalar in inject overwrites base",
			base:   `{"enable_search":false}`,
			inject: `{"enable_search":true}`,
			want:   `{"enable_search":true}`,
		},
		{
			name:   "new key is added",
			base:   `{"model":"m"}`,
			inject: `{"enable_search":true}`,
			want:   `{"enable_search":true,"model":"m"}`,
		},
		{
			name:   "objects merge recursively",
			base:   `{"a":{"x":1,"y":2}}`,
			inject: `{"a":{"y":3,"z":4}}`,
			want:   `{"a":{"x":1,"y":3,"z":4}}`,
		},
		{
			name:   "arrays concatenate base first",
			base:   `{"tools":[{"a":1}]}`,
			inject: `{"tools":[{"b":2}]}`,
			want:   `{"tools":[{"a":1},{"b":2}]}`,
		},
		{
			name:   "nested arrays inside objects concatenate",
			base:   `{"a":{"list":[1,2]}}`,
			inject: `{"a":{"list":[3]}}`,
			want:   `{"a":{"list":[1,2,3]}}`,
		},
		{
			name:   "inject array replaces a non-array base value",
			base:   `{"a":1}`,
			inject: `{"a":[1,2]}`,
			want:   `{"a":[1,2]}`,
		},
		{
			name:   "inject object replaces a non-object base value",
			base:   `{"a":1}`,
			inject: `{"a":{"b":2}}`,
			want:   `{"a":{"b":2}}`,
		},
		{
			name:   "empty base with inject yields inject",
			base:   ``,
			inject: `{"a":1}`,
			want:   `{"a":1}`,
		},
		{
			name:   "numbers keep their precision",
			base:   `{"a":1}`,
			inject: `{"b":9007199254740993}`,
			want:   `{"a":1,"b":9007199254740993}`,
		},
		{
			name:   "html characters are not escaped",
			base:   `{"a":1}`,
			inject: `{"url":"https://x.test/a?b=1&c=2"}`,
			want:   `{"a":1,"url":"https://x.test/a?b=1&c=2"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Inject(json.RawMessage(tt.base), json.RawMessage(tt.inject))
			if err != nil {
				t.Fatalf("Inject() error = %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("Inject() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestInjectErrors(t *testing.T) {
	tests := []struct {
		name   string
		base   string
		inject string
	}{
		{name: "inject is not valid JSON", base: `{"a":1}`, inject: `{oops`},
		{name: "inject is a JSON array", base: `{"a":1}`, inject: `[1,2]`},
		{name: "inject is a scalar", base: `{"a":1}`, inject: `1`},
		{name: "inject is null", base: `{"a":1}`, inject: `null`},
		{name: "base is a JSON array", base: `[1,2]`, inject: `{"a":1}`},
		{name: "base is not valid JSON", base: `{oops`, inject: `{"a":1}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Inject(json.RawMessage(tt.base), json.RawMessage(tt.inject)); err == nil {
				t.Fatalf("Inject(%s, %s) = nil error, want error", tt.base, tt.inject)
			}
		})
	}
}
