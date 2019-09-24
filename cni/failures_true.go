// +build failures

package cni

import (
	"os"
	"path/filepath"
)

const injectFailures = true

// panicForFirstRun panics at least for the first run.
// This "first" is judged against the lifetime of "/tmp" files.
// This does not consider race conditions.
func panicForFirstRun(name string) {
	fileName := filepath.Join("/tmp", "coil_failures_"+name)

	_, err := os.Stat(fileName)
	if err == nil {
		return
	}
	if !os.IsNotExist(err) {
		panic(err)
	}

	_, err = os.Create(fileName)
	if err != nil {
		panic(err)
	}
	panic("injected failure: " + name)
}
