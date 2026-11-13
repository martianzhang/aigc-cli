package argutil

import "testing"

func FuzzSplit(f *testing.F) {
	f.Add("-crf 20")
	f.Add(`-vf "scale=100:100"`)
	f.Add(`-metadata "title=a b" -crf 18`)
	f.Add("   ")
	f.Add(`"unterminated`)

	f.Fuzz(func(t *testing.T, s string) {
		args := Split(s)
		for _, arg := range args {
			if arg == "" {
				t.Fatalf("Split(%q) returned an empty argument: %#v", s, args)
			}
		}
	})
}
