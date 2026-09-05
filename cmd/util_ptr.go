package cmd

// deref returns the value pointed to by p, or def when p is nil.
func deref[T any](p *T, def T) T {
	if p == nil {
		return def
	}
	return *p
}

// field applies f to p when p is non-nil, otherwise returns the zero value.
// Compose field calls to navigate a chain of optional pointers without
// writing nested nil checks:
//
//	defaults := field(shared.Cfg, func(c *types.Config) *types.ConfigDefaults { return c.Defaults })
//	chat    := field(defaults, func(d *types.ConfigDefaults) *types.ChatDefaults { return d.Chat })
func field[T, R any](p *T, f func(*T) R) R {
	if p == nil {
		var zero R
		return zero
	}
	return f(p)
}
