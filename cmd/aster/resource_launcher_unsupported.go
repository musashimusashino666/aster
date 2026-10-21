//go:build !linux

package main

import "fmt"

func commandResourceLauncher(args []string) error {
	return fmt.Errorf("internal resource launcher is only supported on Linux")
}
