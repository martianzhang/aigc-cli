//go:build darwin || linux || freebsd

package cmd

// silenceCAPI and loudCAPI are no-ops. C library debug output is suppressed
// via Debug: 0 in model configs. If the library prints additional info to
// stderr (e.g. resampler messages), it cannot be suppressed portably.
func silenceCAPI() {}
func loudCAPI()    {}
