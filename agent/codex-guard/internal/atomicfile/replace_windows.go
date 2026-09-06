//go:build windows

package atomicfile

import "golang.org/x/sys/windows"

func Replace(temporaryPath, destinationPath string) error {
	return windows.Rename(temporaryPath, destinationPath)
}
