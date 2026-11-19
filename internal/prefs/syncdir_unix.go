//go:build unix

package prefs

import (
	"errors"
	"os"
)

func syncSettingsDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	return errors.Join(dir.Sync(), dir.Close())
}
