package config

import (
	"os"
	"path/filepath"
)

func defaultDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".pdf-translator")
	}
	return filepath.Join(home, ".pdf-translator")
}
