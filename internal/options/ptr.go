package options

// Deref returns the value pointed to by p, or def when p is nil.
func Deref[T any](p *T, def T) T {
	if p == nil {
		return def
	}
	return *p
}

// Field applies f to p when p is non-nil, otherwise returns the zero value.
// Compose Field calls to navigate a chain of optional pointers without writing
// nested nil checks.
func Field[T, R any](p *T, f func(*T) R) R {
	if p == nil {
		var zero R
		return zero
	}
	return f(p)
}
