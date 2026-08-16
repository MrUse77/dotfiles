#!/bin/bash
# Emite el brillo actual cada segundo para eww (deflisten).
# La primera línea sale al instante, evitando el frame inicial vacío.
# Sin backlight (desktop sin control), emite 0.
while true; do
    val="$(brightnessctl -m 2>/dev/null | grep -m1 backlight | cut -d, -f4 | tr -d '%')"
    echo "${val:-0}"
    sleep 1
done
