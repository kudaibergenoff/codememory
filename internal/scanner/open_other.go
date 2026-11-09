//go:build !darwin && !linux

package scanner

import "os"

func openFile(root *os.Root, name string) (*os.File, error) { return root.Open(name) }
