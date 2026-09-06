//go:build !windows

package atomicfile

import "os"

func Replace(temporaryPath, destinationPath string) error {
	return os.Rename(temporaryPath, destinationPath)
}
