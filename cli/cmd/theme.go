package cmd

import (
	"fmt"
	"os"

	"github.com/MrUse77/dots-cli/pkg/theme"
	"github.com/spf13/cobra"
)

var themeCmd = &cobra.Command{
	Use:   "theme [name]",
	Short: "Aplica un tema a Terminal, Wallpaper y Barra (Paso 1)",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		themeName := args[0]
		fmt.Printf("🔧 Cargando tema '%s' (Dummy)...\n", themeName)

		// Dummy Dracula theme con rutas a archivos fijos
		dracula := theme.Theme{
			Name:      "Dracula",
			Wallpaper: "/ruta/a/dracula/wallpaper.png",
			Ghostty:   "/ruta/a/dracula/ghostty_colors",
			Waybar:    "/ruta/a/dracula/waybar_colors.css",
			Yazi:      "/ruta/a/dracula/yazi_theme.toml",
		}

		fmt.Printf("🚀 Aplicando tema '%s' a la Terminal...\n", themeName)
		if err := theme.ApplyGhostty(dracula.Ghostty); err != nil {
			fmt.Fprintf(os.Stderr, "Error applying Ghostty theme: %v\n", err)
		}

		fmt.Printf("🖼️  Aplicando tema '%s' al Wallpaper...\n", themeName)
		if err := theme.ApplyHyprpaper(dracula.Wallpaper); err != nil {
			fmt.Fprintf(os.Stderr, "Error applying Hyprpaper wallpaper: %v\n", err)
		}

		fmt.Printf("📊 Aplicando tema '%s' a Waybar...\n", themeName)
		if err := theme.ApplyWaybar(dracula.Waybar); err != nil {
			fmt.Fprintf(os.Stderr, "Error applying Waybar theme: %v\n", err)
		}

		fmt.Printf("📁 Aplicando tema '%s' a Yazi...\n", themeName)
		if err := theme.ApplyYazi(dracula.Yazi); err != nil {
			fmt.Fprintf(os.Stderr, "Error applying Yazi theme: %v\n", err)
		}

		fmt.Println("✅ ¡Paso 1 completado!")
	},
}

func init() {
	rootCmd.AddCommand(themeCmd)
}
