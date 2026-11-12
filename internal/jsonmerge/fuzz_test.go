package jsonmerge

import (
	"encoding/json"
	"testing"
)

func FuzzInject(f *testing.F) {
	f.Add([]byte(`{"a":1,"nested":{"x":true}}`), []byte(`{"nested":{"y":[1,2]}}`))
	f.Add([]byte(`{}`), []byte(`{"a":null}`))
	f.Add([]byte(`not json`), []byte(`{"a":1}`))
	f.Add([]byte(``), []byte(`[]`))

	f.Fuzz(func(t *testing.T, base, inject []byte) {
		// An empty inject is the documented no-op and deliberately returns the
		// base bytes unchanged, so only validate when a merge actually ran.
		if len(inject) == 0 {
			return
		}
		got, err := Inject(base, inject)
		if err != nil {
			return
		}
		// Anything returned as success must be valid JSON.
		if !json.Valid(got) {
			t.Fatalf("Inject() returned invalid JSON: %q", got)
		}
	})
}
