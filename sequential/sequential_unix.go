//go:build !windows
// +build !windows

package sequential

import "os"

var (
	Create     = os.Create
	Open       = os.Open
	OpenFile   = os.OpenFile
	CreateTemp = os.CreateTemp
)
