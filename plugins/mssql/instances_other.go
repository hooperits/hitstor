//go:build !windows

package main

// localInstances finds no instances outside Windows; the configured
// instance is listed instead.
func localInstances() []instanceInfo { return nil }
