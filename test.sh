#!/bin/bash

# Este script automatiza la creación y ejecución del entorno de pruebas.

echo "================================================="
echo "  🛠️  Construyendo entorno de pruebas (Arch Linux) "
echo "================================================="

# Construye la imagen usando el Dockerfile.test
docker build -t dotfiles-tester -f Dockerfile.test .

echo ""
echo "================================================="
echo "  🚀 Levantando el contenedor aislado..."
echo "================================================="

# Ejecuta el contenedor:
# -it: Modo interactivo con terminal.
# --rm: Destruye el contenedor al salir (no deja basura en tu PC).
# -v: Monta tu código actual en el contenedor en tiempo real.
docker run -it --rm \
    -v "$(pwd)":/home/tester/dotfiles \
    --name dotfiles-sandbox \
    dotfiles-tester
