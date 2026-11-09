//go:build darwin || linux

package scanner

import (
	"os"
	"syscall"
)

// Do not block if a regular file is swapped for a FIFO before open.
func openFile(root *os.Root, name string) (*os.File, error) {
	return root.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
}
