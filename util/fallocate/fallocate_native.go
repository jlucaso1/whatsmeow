//go:build !wasm

package fallocate

import (
	"fmt"
	"os"

	goFallocate "go.mau.fi/util/fallocate"
	"go.mau.fi/whatsmeow/iface"
)

func Preallocate(w iface.File, size int64) error {
	file, ok := w.(*os.File)
	if !ok {
		return fmt.Errorf("underlying file type is not *os.File, cannot fallocate")
	}

	return goFallocate.Fallocate(file, int(size))
}
