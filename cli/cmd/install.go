package cmd

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/MrUse77/dots-cli/pkg/installer"
	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
)

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Copia toda la configuración del repo al sistema (~/.config)",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Bienvenido al instalador de dotfiles")

		var confirm bool
		err := huh.NewForm(
			huh.NewGroup(
				huh.NewConfirm().
					Title("¿Estás seguro que querés modificar tu sistema?").
					Value(&confirm),
			),
		).Run()

		if err != nil || !confirm {
			fmt.Println("Instalación cancelada.")
			os.Exit(0)
		}

		var mode string
		var hasAMD bool
		var installPlugins bool

		err = huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("Modo de instalación").
					Options(
						huh.NewOption("Modo Usuario (Copia limpia, no se sincroniza con Git)", "user"),
						huh.NewOption("Modo Dev (Symlinks con Stow, ideal para seguir editando)", "dev"),
					).
					Value(&mode),
				huh.NewConfirm().
					Title("¿Tenés GPU AMD? (Instalará corectrl)").
					Value(&hasAMD),
				huh.NewConfirm().
					Title("¿Instalar plugins de Hyprland via hyprpm?").
					Description("Requiere que Hyprland esté corriendo.").
					Value(&installPlugins),
			),
		).Run()

		if err != nil {
			fmt.Println("Instalación cancelada.")
			os.Exit(0)
		}

		fmt.Printf("\nOpciones elegidas:\n- Modo: %s\n- GPU AMD: %v\n- Plugins Hyprland: %v\n\n", mode, hasAMD, installPlugins)

		if mode == "dev" {
			fmt.Println("Ejecutando en Modo Dev (Stow) - Próximamente...")
			return
		}

		fmt.Println("🚀 Iniciando instalación de dependencias...")
		if err := installer.UpdateAndInstallBase(); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Error actualizando base: %v\n", err)
			return
		}
		if err := installer.InstallParu(); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Error instalando paru: %v\n", err)
			return
		}
		if err := installer.InstallPackages(hasAMD); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Error instalando paquetes: %v\n", err)
			return
		}
		if err := installer.SetupShell(); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Error configurando shell: %v\n", err)
		}

		homeDir, _ := os.UserHomeDir()
		repoRoot := ".."

		fmt.Println("📦 Inicializando submódulos de Git...")
		if err := installer.InitSubmodules(repoRoot); err != nil {
			fmt.Fprintf(os.Stderr, "⚠️  Error en submódulos: %v\n", err)
		}

		repoConfig := filepath.Join(repoRoot, ".config")
		systemConfig := filepath.Join(homeDir, ".config")

		// Respaldar configs existentes que puedan generar conflictos
		fmt.Println("🔒 Respaldando configuraciones conflictivas...")
		if err := backupConflicts(repoConfig, systemConfig); err != nil {
			fmt.Fprintf(os.Stderr, "⚠️  Error en backup: %v\n", err)
		}

		fmt.Println("🚀 Iniciando despliegue de dotfiles (Modo Usuario)...")
		fmt.Printf("Copiando desde %s hacia %s\n", repoConfig, systemConfig)

		err = copyDir(repoConfig, systemConfig)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ Error copiando config: %v\n", err)
			return
		}

		// Copiar archivos sueltos en el root
		itemsToCopy := []string{".zshrc", ".gtkrc-2.0", "oh-my-posh", ".zsh_plugins", ".themes"}
		for _, item := range itemsToCopy {
			src := filepath.Join(repoRoot, item)
			dst := filepath.Join(homeDir, item)
			
			// Detectar si es directorio o archivo para usar copyDir o copyFileInternal
			info, err := os.Stat(src)
			if err != nil {
				continue // Ignorar si no existe
			}
			
			if info.IsDir() {
				if err := copyDir(src, dst); err == nil {
					fmt.Printf("✅ Copiado %s/\n", item)
				}
			} else {
				if err := copyFileInternal(src, dst); err == nil {
					fmt.Printf("✅ Copiado %s\n", item)
				}
			}
		}

		if err := installer.InstallFontsAndCursors(repoRoot); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Error instalando fuentes y cursores: %v\n", err)
		}
		if err := installer.ApplyGSettings(); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Error aplicando GSettings: %v\n", err)
		}
		if err := installer.EnableServices(); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Error habilitando servicios: %v\n", err)
		}
		if err := installer.SetupEnvVars(); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Error configurando variables de entorno: %v\n", err)
		}
		if err := installer.EnsureZshDirs(); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Error creando directorios zsh: %v\n", err)
		}

		if installPlugins {
			fmt.Println("🔌 Instalando plugins de Hyprland...")
			if err := installer.InstallHyprlandPlugins(); err != nil {
				fmt.Fprintf(os.Stderr, "❌ Error instalando plugins: %v\n", err)
			}
		}

		fmt.Println("🎉 ¡Configuración desplegada exitosamente!")
	},
}

func init() {
	rootCmd.AddCommand(installCmd)
}

// copyDir copia un directorio recursivamente
func copyDir(src string, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Ignorar la carpeta .git si existe (como en .config/nvim)
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}

		relPath, _ := filepath.Rel(src, path)
		destPath := filepath.Join(dst, relPath)

		if d.IsDir() {
			return os.MkdirAll(destPath, 0755)
		}

		return copyFileInternal(path, destPath)
	})
}

// backupConflicts renombra a .bak los archivos/carpetas que serían sobreescritos
func backupConflicts(repoConfig, systemConfig string) error {
	entries, err := os.ReadDir(repoConfig)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		target := filepath.Join(systemConfig, entry.Name())
		if _, err := os.Lstat(target); err != nil {
			continue
		}
		if info, _ := os.Lstat(target); info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		bak := target + ".bak"
		fmt.Printf("⚠️  Respaldando %s -> %s\n", target, bak)
		if err := os.Rename(target, bak); err != nil {
			return err
		}
	}
	return nil
}

func copyFileInternal(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}
