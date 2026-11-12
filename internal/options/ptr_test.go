package options

import "testing"

func TestDeref(t *testing.T) {
	cases := []struct {
		name string
		in   *int
		def  int
		want int
	}{
		{"non-nil", ptr(42), 0, 42},
		{"nil-fallback", nil, 7, 7},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Deref(c.in, c.def); got != c.want {
				t.Errorf("Deref(%v, %d) = %d, want %d", c.in, c.def, got, c.want)
			}
		})
	}
}

func TestField(t *testing.T) {
	type inner struct{ V int }
	type outer struct{ Inner *inner }

	i := &inner{V: 5}
	o := &outer{Inner: i}

	cases := []struct {
		name string
		in   *outer
		want *inner
	}{
		{"both-non-nil", o, i},
		{"nil-root", nil, nil},
		{"nil-child", &outer{}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Field(c.in, func(o *outer) *inner { return o.Inner })
			if got != c.want {
				t.Errorf("Field = %v, want %v", got, c.want)
			}
		})
	}
}

func ptr(v int) *int { return &v }
