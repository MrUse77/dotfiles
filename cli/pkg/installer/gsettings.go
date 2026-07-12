package installer

import (
	"fmt"
)

func ApplyGSettings() error {
	fmt.Println("Aplicando temas GTK via gsettings...")
	
	settings := []struct {
		key   string
		value string
	}{
		{"gtk-theme", "TokyoNight-zk"},
		{"icon-theme", "TokyoNight-SE"},
		{"cursor-theme", "volantes_cursors"},
		{"cursor-size", "24"},
		{"font-name", "CaskaydiaMono Nerd Font Mono Bold 10"},
		{"color-scheme", "prefer-dark"},
	}

	for _, s := range settings {
		val := s.value
		// No necesitamos agregar las comillas en Go porque os/exec pasa el string directamente, 
		// pero gsettings a veces requiere strings de GVariant válidos.
		// En bash `gsettings set ... font-name "Algo"` funciona porque bash quita las comillas
		// y pasa 'Algo' como un solo argumento y gsettings lo acepta.
		// Si el tipo es estrictamente string y falla, gsettings recomienda poner comillas simples alrededor, 
		// pero si la CLI de gsettings lo parsea bien sin ellas como un solo argumento, probemos enviarlo directo.
		// En caso de que falle por ser un string gvariant estricto, le agregamos comillas. 
		// En base al script bash original, simplemente pasa el valor sin comillas adicionales.
		if err := runCommand("gsettings", "set", "org.gnome.desktop.interface", s.key, val); err != nil {
			return fmt.Errorf("error configurando gsettings %s: %w", s.key, err)
		}
	}
	return nil
}
