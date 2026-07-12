package theme

import (
	"os"
	"path/filepath"
)

// ApplyGhostty copia el archivo de colores de Ghostty
func ApplyGhostty(themePath string) error {
	if themePath == "" {
		return nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	// Usamos la carpeta aislada de Dev
	destPath := filepath.Join(homeDir, "Dev", "dotfiles", ".config", "ghostty", "config")

	return copyFile(themePath, destPath)
}
