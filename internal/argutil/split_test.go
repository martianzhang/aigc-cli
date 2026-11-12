package argutil

import (
	"reflect"
	"testing"
)

func TestSplit(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"  ", nil},
		{"-crf 20", []string{"-crf", "20"}},
		{`-vf "scale=100:100"`, []string{"-vf", "scale=100:100"}},
		{`-metadata "title=hello world" -crf 18`, []string{"-metadata", "title=hello world", "-crf", "18"}},
	}
	for _, tc := range tests {
		if got := Split(tc.in); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("Split(%q) = %#v, want %#v", tc.in, got, tc.want)
		}
	}
}
