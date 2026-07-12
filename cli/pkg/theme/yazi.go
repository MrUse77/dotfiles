package theme

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// ApplyYazi copia el archivo theme.toml proporcionado a la carpeta de configuración de Yazi.
func ApplyYazi(themePath string) error {
	if themePath == "" {
		return nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("could not get home directory: %v", err)
	}

	// Yazi en Dev/dotfiles (para testing seguro)
	destPath := filepath.Join(homeDir, "Dev", "dotfiles", ".config", "yazi", "theme.toml")

	srcFile, err := os.Open(themePath)
	if err != nil {
		return fmt.Errorf("failed to open source yazi theme %s: %v", themePath, err)
	}
	defer srcFile.Close()

	destFile, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create yazi theme file %s: %v", destPath, err)
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, srcFile); err != nil {
		return fmt.Errorf("failed to copy yazi theme content: %v", err)
	}

	return nil
}
