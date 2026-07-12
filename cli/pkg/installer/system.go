package installer

import (
	"fmt"
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
	
	if err := runCommand("sh", "-c", fmt.Sprintf("cp -r %s/* %s/", filepath.Join(dotfilesDir, "assets", "fonts"), fontsDir)); err != nil {
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
	if err := runCommand("sh", "-c", fmt.Sprintf("cp -r %s/* %s/", filepath.Join(dotfilesDir, "assets", "icons"), iconsDir)); err != nil {
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
		"/etc/profile.d/qt-theme.sh":    qtTheme,
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

