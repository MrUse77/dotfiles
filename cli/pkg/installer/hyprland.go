package installer

import "fmt"

// InstallHyprlandPlugins runs hyprpm to install and enable the configured Hyprland plugins.
func InstallHyprlandPlugins() error {
	fmt.Println("Inicializando hyprpm...")

	if err := runCommand("hyprpm", "update"); err != nil {
		fmt.Printf("⚠️  hyprpm update falló (Hyprland debe estar corriendo para esto): %v\n", err)
	}

	if err := runCommand("hyprpm", "add", "https://github.com/hyprwm/hyprland-plugins"); err != nil {
		fmt.Printf("⚠️  Fallo al agregar repo de plugins: %v\n", err)
	}

	if err := runCommand("hyprpm", "enable", "hyprbars"); err != nil {
		fmt.Printf("⚠️  No se pudo habilitar hyprbars: %v\n", err)
	} else {
		fmt.Println("✅ Plugin hyprbars habilitado.")
	}

	if err := runCommand("hyprpm", "add", "https://github.com/zjeffer/split-monitor-workspaces"); err != nil {
		fmt.Printf("⚠️  Fallo al agregar split-monitor-workspaces: %v\n", err)
	}
	if err := runCommand("hyprpm", "enable", "split-monitor-workspaces"); err != nil {
		fmt.Printf("⚠️  No se pudo habilitar split-monitor-workspaces: %v\n", err)
	} else {
		fmt.Println("✅ Plugin split-monitor-workspaces habilitado.")
	}

	return nil
}
