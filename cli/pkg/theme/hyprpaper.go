package theme

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

func ApplyHyprpaper(wallpaper string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	destPath := filepath.Join(home, ".config", "hypr", "assets", "wallpaper.png")

	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return err
	}

	src, err := os.Open(wallpaper)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return err
	}

	cmd := exec.Command("hyprctl", "hyprpaper", "wallpaper", ","+destPath)
	if err := cmd.Run(); err != nil {
		fmt.Printf("Warning: failed to execute hyprpaper reload: %v\n", err)
	}

	return nil
}
