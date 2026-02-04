//go:build !windows

package main

func uiStartupGuard() error {
	return nil
}

func uiStartupAlert(err error) {
}
