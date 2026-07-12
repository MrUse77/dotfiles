package theme

// Theme representa las rutas a los archivos de configuración de un tema
type Theme struct {
	Name      string `json:"name"`
	Wallpaper string `json:"wallpaper"` // Ruta a la imagen
	Ghostty   string `json:"ghostty"`   // Ruta al archivo de colores de Ghostty
	Waybar    string `json:"waybar"`    // Ruta al archivo colors.css de Waybar
	Yazi      string `json:"yazi"`      // Ruta al archivo theme.toml de Yazi
}
