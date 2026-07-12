package theme

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

// ApplyWaybar copia el colors.css y recarga Waybar
func ApplyWaybar(themePath string) error {
	if themePath == "" {
		return nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	destPath := filepath.Join(homeDir, "Dev", "dotfiles", ".config", "waybar", "colors.css")

	if err := copyFile(themePath, destPath); err != nil {
		return err
	}

	// Recargar waybar
	cmd := exec.Command("killall", "-SIGUSR2", "waybar")
	// cmd.Run() // Comentado para evitar efectos secundarios en vivo
	_ = cmd

	return nil
}

// Utilidad interna para copiar archivos
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open %s: %v", src, err)
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create %s: %v", dst, err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("failed to copy content: %v", err)
	}
	return nil
}
