//go:build !embedded

package main

func SetKubeAPIServerConfigFromGlobals(_, _, _ string) error {
	return nil
}
