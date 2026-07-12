# TUI Installer Design

## Overview
Replace the `install.sh` shell script interactive prompts (which used `gum`) with a native Go TUI using Charmbracelet's `huh` library.

## Requirements
Replicate the following flow from `install.sh`:
1. Welcome message.
2. Confirmation prompt: "¿Estás seguro que querés modificar tu sistema?" (If no, exit 0).
3. Select prompt: "Modo de instalación"
   - Option A: "Modo Usuario (Copia limpia, no se sincroniza con Git)"
   - Option B: "Modo Dev (Symlinks con Stow, ideal para seguir editando)"
4. Confirmation prompt: "¿Tenés GPU AMD? (Instalará corectrl)"

## Architecture
- Use `github.com/charmbracelet/huh` for the interactive forms.
- Update `cmd/install.go` to display this form before executing the file copying logic.
- The user's choices will dictate the execution (e.g., if User Mode, do `copyDir`; if Dev Mode, run `stow`).
