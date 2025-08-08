//go:build wasm

package fallocate

import "go.mau.fi/whatsmeow/iface"

// Preallocate is a no-op in the WASM environment, as browser APIs
// do not support file pre-allocation. This function exists to satisfy
// the Go compiler for WASM builds.
func Preallocate(w iface.File, size int64) error {
	// In a browser context, pre-allocation is not applicable.
	// We simply do nothing and return success.
	return nil
}
