//go:build wasm

package fallocate

import "go.mau.fi/whatsmeow/iface"

func Preallocate(w iface.File, size int64) error {
	return nil
}
