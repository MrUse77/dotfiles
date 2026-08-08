#!/bin/bash

function emit_vol() {
    vol=$(pamixer --get-volume)
    mute=$(pamixer --get-mute)
    if [ "$mute" = "true" ]; then
        # Enviamos un JSON para actualizar icono y volumen de un solo tiro
        echo "{\"volume\": 0, \"icon\": \"󰖁\"}"
    else
        echo "{\"volume\": $vol, \"icon\": \"󰕾\"}"
    fi
}

# Emisión inicial inmediata
emit_vol

# Escuchar cambios
pactl subscribe | stdbuf -oL grep --line-buffered "Event 'change' on sink" | while read -r _; do
    emit_vol
done