package mcp

import (
	"testing"

	"github.com/martianzhang/aigc-cli/internal/annotate"
)

func TestParseAnnotate(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want annotate.Options
	}{
		{"empty", "", annotate.Options{}},
		{"skeleton only", "skeleton", annotate.Options{Skeleton: true}},
		{"face only", "face", annotate.Options{Face: true}},
		{"both comma", "skeleton,face", annotate.Options{Skeleton: true, Face: true}},
		{"case insensitive", "Face,Skeleton", annotate.Options{Skeleton: true, Face: true}},
		{"unknown ignored", "foo", annotate.Options{}},
		{"mixed with space", " skeleton , face ", annotate.Options{Skeleton: true, Face: true}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseAnnotate(c.in)
			if got != c.want {
				t.Errorf("parseAnnotate(%q) = %+v, want %+v", c.in, got, c.want)
			}
		})
	}
}

func TestConvertDepthSchemaHasAnnotate(t *testing.T) {
	tool := newConvertDepthTool()
	if _, ok := tool.InputSchema.Properties["annotate"]; !ok {
		t.Fatal("convert_depth schema missing annotate param")
	}
	if _, ok := tool.InputSchema.Properties["input_path"]; !ok {
		t.Fatal("convert_depth schema missing input_path param")
	}
}
