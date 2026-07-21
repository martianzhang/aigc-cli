//go:build !darwin && !linux && !freebsd

package cmd

func silenceCAPI() {}
func loudCAPI()    {}
