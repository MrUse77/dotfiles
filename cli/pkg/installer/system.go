package installer

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func SetupShell() error {
	fmt.Println("Cambiando shell por defecto a zsh...")
	zshPath, err := exec.LookPath("zsh")
	if err != nil {
		return fmt.Errorf("zsh no encontrado: %w", err)
	}

	shell := os.Getenv("SHELL")
	if shell != zshPath {
		if err := runCommand("chsh", "-s", zshPath); err != nil {
			return fmt.Errorf("error cambiando shell: %w", err)
		}
		fmt.Println("Shell cambiado a zsh. Reiniciá sesión para que tome efecto.")
		return nil
	}
	fmt.Println("zsh ya es el shell por defecto.")
	return nil
}

func copyDirectoryContents(source, destination string) error {
	entries, err := os.ReadDir(source)
	if err != nil {
		return fmt.Errorf("read directory %q: %w", source, err)
	}
	for _, entry := range entries {
		if err := copyPath(filepath.Join(source, entry.Name()), filepath.Join(destination, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func copyPath(source, destination string) error {
	if err := rejectDestinationSymlinks(destination); err != nil {
		return err
	}
	info, err := os.Lstat(source)
	if err != nil {
		return fmt.Errorf("stat %q: %w", source, err)
	}
	if info.IsDir() {
		if err := os.MkdirAll(destination, info.Mode().Perm()); err != nil {
			return fmt.Errorf("mkdir %q: %w", destination, err)
		}
		entries, err := os.ReadDir(source)
		if err != nil {
			return fmt.Errorf("read directory %q: %w", source, err)
		}
		for _, entry := range entries {
			if err := copyPath(filepath.Join(source, entry.Name()), filepath.Join(destination, entry.Name())); err != nil {
				return err
			}
		}
		return os.Chmod(destination, info.Mode().Perm())
	}
	if info.Mode()&os.ModeSymlink != 0 {
		link, err := os.Readlink(source)
		if err != nil {
			return fmt.Errorf("read symlink %q: %w", source, err)
		}
		if err := os.Symlink(link, destination); err != nil {
			return fmt.Errorf("create symlink %q: %w", destination, err)
		}
		return nil
	}
	input, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open %q: %w", source, err)
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return fmt.Errorf("create %q: %w", destination, err)
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return fmt.Errorf("copy %q: %w", source, copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close %q: %w", destination, closeErr)
	}
	return nil
}

func rejectDestinationSymlinks(destination string) error {
	current := filepath.Clean(destination)
	for {
		info, err := os.Lstat(current)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("refusing destination symlink %q", current)
			}
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect destination %q: %w", current, err)
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return nil
}

func InstallFontsAndCursors(dotfilesDir string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("error obteniendo home: %w", err)
	}

	fmt.Println("Instalando fuentes y caché...")
	fontsDir := filepath.Join(homeDir, ".local", "share", "fonts")
	if err := os.MkdirAll(fontsDir, 0755); err != nil {
		return fmt.Errorf("error creando directorio de fuentes: %w", err)
	}

	if err := copyDirectoryContents(filepath.Join(dotfilesDir, "assets", "fonts"), fontsDir); err != nil {
		return fmt.Errorf("error copiando fuentes: %w", err)
	}
	if err := runCommand("fc-cache", "-f"); err != nil {
		return fmt.Errorf("error actualizando caché de fuentes: %w", err)
	}

	fmt.Println("Instalando cursor theme...")
	iconsDir := filepath.Join(homeDir, ".local", "share", "icons")
	if err := os.MkdirAll(iconsDir, 0755); err != nil {
		return fmt.Errorf("error creando directorio de iconos: %w", err)
	}
	if err := copyDirectoryContents(filepath.Join(dotfilesDir, "assets", "icons"), iconsDir); err != nil {
		return fmt.Errorf("error copiando cursores: %w", err)
	}

	return nil
}

func EnableServices() error {
	fmt.Println("Habilitando servicios...")

	if err := exec.Command("systemctl", "is-active", "--quiet", "tlp").Run(); err == nil {
		fmt.Println("TLP detectado y activo. Omitiendo power-profiles-daemon para evitar conflictos.")
	} else {
		if err := runCommand("sudo", "systemctl", "enable", "--now", "power-profiles-daemon.service"); err != nil {
			return fmt.Errorf("error habilitando power-profiles-daemon: %w", err)
		}
	}

	if err := runCommand("sudo", "systemctl", "enable", "--now", "upower"); err != nil {
		return fmt.Errorf("error habilitando upower: %w", err)
	}
	return nil
}

// SetupEnvVars escribe las variables de entorno de Qt y Wayland en /etc/profile.d/
func SetupEnvVars() error {
	fmt.Println("Configurando variables de entorno en /etc/profile.d/...")

	qtTheme := `export QT_QPA_PLATFORMTHEME=qt5ct
export QT_STYLE_OVERRIDE=kvantum
`
	waylandVars := `export MOZ_ENABLE_WAYLAND=1
export GDK_BACKEND=wayland,x11,*
`

	files := map[string]string{
		"/etc/profile.d/qt-theme.sh":     qtTheme,
		"/etc/profile.d/wayland-vars.sh": waylandVars,
	}

	for path, content := range files {
		cmd := exec.Command("sudo", "tee", path)
		cmd.Stdin = strings.NewReader(content)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("error escribiendo %s: %w", path, err)
		}
		if err := runCommand("sudo", "chmod", "+x", path); err != nil {
			return fmt.Errorf("error aplicando permisos a %s: %w", path, err)
		}
	}

	fmt.Println("✅ Variables de entorno configuradas.")
	return nil
}

// InitSubmodules inicializa los submódulos de Git (plugins de Neovim, zsh, etc.)
func InitSubmodules(dotfilesDir string) error {
	fmt.Println("Inicializando submódulos de Git...")
	cmd := exec.Command("git", "submodule", "update", "--init", "--recursive")
	cmd.Dir = dotfilesDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error inicializando submódulos: %w", err)
	}
	fmt.Println("✅ Submódulos inicializados.")
	return nil
}

// EnsureZshDirs crea los directorios de caché e historial de zsh
func EnsureZshDirs() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	zshDir := filepath.Join(homeDir, ".config", "zsh")
	if err := os.MkdirAll(zshDir, 0755); err != nil {
		return fmt.Errorf("error creando ~/.config/zsh: %w", err)
	}
	fmt.Println("✅ Directorio ~/.config/zsh listo.")
	return nil
}

// EnableSSHAgent habilita ssh-agent como servicio de systemd --user
// y exporta SSH_AUTH_SOCK via /etc/profile.d/
func EnableSSHAgent() error {
	fmt.Println("Habilitando ssh-agent via systemd --user...")

	if err := runCommand("systemctl", "--user", "enable", "--now", "ssh-agent"); err != nil {
		return fmt.Errorf("error habilitando ssh-agent.service: %w", err)
	}

	content := "# ssh-agent managed by systemd --user\nexport SSH_AUTH_SOCK=\"$XDG_RUNTIME_DIR/ssh-agent.socket\"\n"
	cmd := exec.Command("sudo", "tee", "/etc/profile.d/ssh-agent.sh")
	cmd.Stdin = strings.NewReader(content)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error escribiendo /etc/profile.d/ssh-agent.sh: %w", err)
	}
	if err := runCommand("sudo", "chmod", "+x", "/etc/profile.d/ssh-agent.sh"); err != nil {
		return err
	}

	fmt.Println("✅ SSH Agent habilitado. Reiniciá sesión para que tome efecto.")
	return nil
}
