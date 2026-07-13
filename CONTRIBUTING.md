# Contribuir a dotfiles

Guía para humanos y agentes de IA que contribuyan a este repositorio.

> **Regla de oro:** Este repo maneja la configuración real de un sistema.
> Cualquier cambio mal hecho puede romper el entorno del usuario.
> Actuá con precaución, seguí el proceso, y nunca hagas cambios destructivos sin revisión.

---

## Tabla de contenidos

- [Estructura del proyecto](#estructura-del-proyecto)
- [Flujo de trabajo con Git](#flujo-de-trabajo-con-git)
- [Convención de branches](#convención-de-branches)
- [Convención de commits](#convención-de-commits)
- [Pull Requests](#pull-requests)
- [Scopes y límites](#scopes-y-límites)
- [Reglas para agentes de IA](#reglas-para-agentes-de-ia)
- [Submodules](#submodules)
- [Testing](#testing)
- [Estilo y convenciones de código](#estilo-y-convenciones-de-código)

---

## Estructura del proyecto

Este repo tiene **dos ámbitos** con reglas distintas:

| Ámbito | Path | Tipo | Testing |
|---|---|---|---|
| **Config** | `.config/`, `.zshrc`, `.themes/`, `oh-my-posh/`, `assets/` | Archivos de configuración | `bash test.sh` (Docker) |
| **CLI** | `cli/` | Proyecto Go (Cobra + Bubbletea) | `go test ./...` + `bash test.sh` |

Un cambio **nunca** debería mezclar ambos ámbitos en un mismo PR sin justificación explícita.

---

## Flujo de trabajo con Git

```
main (protegida)
 └── feat/descripcion-corta     ← desarrollo
 └── fix/descripcion-corta      ← correcciones
 └── docs/descripcion-corta     ← documentación
 └── chore/descripcion-corta    ← mantenimiento
```

### Reglas

1. **`main` es la rama protegida.** No se pushea directo a `main`. Todo entra por PR.
2. **Una branch por cambio.** Cada branch representa un cambio atómico y autocontenido.
3. **Branches de vida corta.** Merge rápido, delete después del merge.
4. **Rebase antes de merge.** Mantené la historia lineal:
   ```bash
   git fetch origin
   git rebase origin/main
   ```
5. **No force push a branches compartidas.** Solo a branches personales.

---

## Convención de branches

```
<tipo>/<descripcion-kebab-case>
```

### Tipos permitidos

| Tipo | Uso | Ejemplo |
|---|---|---|
| `feat/` | Nueva funcionalidad | `feat/waybar-battery-module` |
| `fix/` | Corrección de bug | `fix/hyprland-monitor-config` |
| `docs/` | Documentación | `docs/update-readme-stack` |
| `chore/` | Mantenimiento, refactors, CI | `chore/update-nvim-submodule` |
| `style/` | Cambios visuales/estéticos | `style/waybar-tokyonight-colors` |
| `test/` | Agregar o mejorar tests | `test/installer-rollback` |

### Ejemplos inválidos

```
# ❌ MAL
mi-branch
update
new-stuff
agente/cambios-varios

# ✅ BIEN
feat/ghostty-font-size
fix/zellij-keybind-conflict
docs/contributing-guide
```

---

## Convención de commits

Usamos [Conventional Commits](https://www.conventionalcommits.org/) en español o inglés.

### Formato

```
<tipo>(<scope>): <descripción imperativa>

[cuerpo opcional]

[footer opcional]
```

### Tipos

| Tipo | Significado |
|---|---|
| `feat` | Nueva funcionalidad |
| `fix` | Corrección de bug |
| `docs` | Solo documentación |
| `style` | Cambios de formato/estilo (no funcionales) |
| `refactor` | Reestructuración sin cambio de comportamiento |
| `test` | Agregar o corregir tests |
| `chore` | Tareas de mantenimiento |

### Scopes válidos

| Scope | Aplica a |
|---|---|
| `hypr` | `.config/hypr/` — Hyprland, hyprlock, hyprpaper, hypridle |
| `waybar` | `.config/waybar/` |
| `ghostty` | `.config/ghostty/` |
| `zellij` | `.config/zellij/` |
| `dunst` | `.config/dunst/` |
| `eww` | `.config/eww/` |
| `wofi` | `.config/wofi/` |
| `yazi` | `.config/yazi/` |
| `nvim` | `.config/nvim/` (submodule) |
| `gtk` | `.config/gtk-3.0/`, `.config/gtk-4.0/`, `.gtkrc-2.0`, `.themes/` |
| `zsh` | `.zshrc`, `.zsh_plugins/` |
| `omp` | `oh-my-posh/` |
| `installer` | `cli/pkg/installer/` |
| `theme` | `cli/pkg/theme/` |
| `cli` | `cli/` (general) |
| `assets` | `assets/` |
| `docker` | `Dockerfile.test`, `test.sh` |
| `openspec` | `openspec/` |

### Ejemplos

```bash
# ✅ Buenos
feat(waybar): add battery module with percentage display
fix(hypr): correct monitor scaling for 1440p
docs(installer): document rollback procedure
chore(zsh): update fzf-tab submodule to latest
refactor(cli): extract package manager detection to separate module

# ❌ Malos
update stuff
fixed things
WIP
changes
feat: many changes across the entire repo
```

### Reglas estrictas

- **Un commit = un cambio lógico.** No metas 5 cosas distintas en un commit.
- **Descripción en imperativo.** "add X", "fix Y", "remove Z" — no "added", "fixing", "removed".
- **Máximo 72 caracteres** en la primera línea.
- **El scope es obligatorio** para cambios en código/config. Solo `docs` y `chore` pueden omitirlo.

---

## Pull Requests

### Título

Mismo formato que los commits:

```
<tipo>(<scope>): <descripción>
```

### Template del cuerpo

```markdown
## Qué cambia

Breve descripción de qué se modificó y por qué.

## Archivos afectados

- `.config/waybar/config.jsonc` — agregar módulo de batería
- `.config/waybar/style.css` — estilos del nuevo módulo

## Testing

- [ ] `bash test.sh` pasa (si aplica)
- [ ] `cd cli && go test ./...` pasa (si hay cambios en cli/)
- [ ] `cd cli && go vet ./...` sin warnings
- [ ] Probado en un entorno real / Docker

## Checklist

- [ ] La branch sigue la convención de nombres
- [ ] Los commits siguen Conventional Commits
- [ ] No se mezclan cambios de config con cambios de CLI sin justificación
- [ ] No se modificaron submodules sin intención (verificar con `git diff --submodule`)
- [ ] No se agregaron archivos binarios innecesarios
- [ ] Los archivos de config son válidos (JSON, TOML, CSS sin errores de sintaxis)
```

### Reglas de merge

1. **Squash merge** para branches con commits WIP o de limpieza.
2. **Merge commit** para branches con historia limpia y significativa.
3. **Borrar la branch** después del merge.
4. **Al menos una revisión** antes de mergear (humano o proceso automatizado).

---

## Scopes y límites

### Qué se puede cambiar

| Acción | ¿Permitida? | Notas |
|---|---|---|
| Agregar un config nuevo en `.config/` | ✅ | Documentar en README si es una tool nueva |
| Modificar valores de config existente | ✅ | Verificar que no rompa defaults |
| Agregar un paquete al instalador | ✅ | Agregar a la lista correspondiente en `cli/` |
| Modificar lógica del CLI (`cli/`) | ✅ | Requiere tests (`go test`) |
| Actualizar un submodule | ✅ | Solo con `git submodule update` |
| Agregar un submodule nuevo | ⚠️ | Requiere justificación en el PR |
| Cambiar la estructura del repo | ⚠️ | Requiere discusión previa |
| Eliminar configs o funcionalidad | ⚠️ | Requiere justificación explícita |

### Qué NO se puede hacer

- ❌ Modificar archivos fuera del repo (no tocar `/etc/`, `~/`, etc. directamente)
- ❌ Agregar secrets, tokens, o credenciales
- ❌ Commitear binarios compilados (el `dots` binary está en `.gitignore` — debería estarlo)
- ❌ Modificar `.gitmodules` sin necesidad
- ❌ Hacer cambios que requieran una versión específica de hardware sin documentar

---

## Reglas para agentes de IA

> Esta sección aplica a todos los agentes de IA (Copilot, Gemini, Claude, etc.)
> que operen sobre este repositorio, ya sea via CLI, IDE, o automatización.

### Principios generales

1. **Seguí el flujo de trabajo.** Branch → commits convencionales → PR. Sin excepciones.
2. **No commitees directo a `main`.** Nunca. Jamás. Bajo ninguna circunstancia.
3. **Un PR = un cambio cohesivo.** No acumules cambios no relacionados.
4. **No hagas cambios especulativos.** Solo cambiá lo que se te pidió.
5. **Preguntá antes de actuar** cuando haya ambigüedad.

### Restricciones hard

| Regla | Descripción |
|---|---|
| **No borrar archivos** sin instrucción explícita del usuario |
| **No modificar submodules** a menos que se pida específicamente |
| **No crear archivos en la raíz** del repo sin aprobación |
| **No modificar `.gitignore`** ni `.gitmodules` sin aprobación |
| **No agregar dependencias** al CLI sin aprobación |
| **No cambiar la estructura** de directorios existente |
| **No hacer refactors masivos** sin discusión previa |
| **No mezclar scopes** (config + CLI) en un mismo PR |

### Convenciones que DEBEN seguir

1. **Branch naming:** `<tipo>/<descripcion-kebab-case>` — ver [sección de branches](#convención-de-branches)
2. **Commits:** Conventional Commits con scope — ver [sección de commits](#convención-de-commits)
3. **PR title:** Mismo formato que commits
4. **Scope boundaries:** Config y CLI son PRs separados
5. **Archivos tocados:** Solo los mínimos necesarios para el cambio

### Antes de hacer un commit

El agente DEBE verificar:

```bash
# 1. Verificar que no hay cambios accidentales en submodules
git diff --submodule

# 2. Verificar que solo se modificaron los archivos esperados
git diff --name-only

# 3. Si hay cambios en cli/, correr tests
cd cli && go test ./... && go vet ./...

# 4. Verificar sintaxis de configs modificados (ejemplo para JSON)
# python3 -m json.tool < archivo.json

# 5. Verificar que el commit message sigue la convención
# tipo(scope): descripción en imperativo, ≤72 chars
```

### Ejemplo de flujo correcto para un agente

```bash
# 1. Crear branch con nombre correcto
git checkout -b feat/waybar-battery-module

# 2. Hacer cambios MÍNIMOS y NECESARIOS
# ... editar archivos ...

# 3. Verificar cambios
git diff --name-only
git diff --submodule  # debe estar vacío si no se tocaron submodules

# 4. Commit con formato correcto
git add .config/waybar/config.jsonc .config/waybar/style.css
git commit -m "feat(waybar): add battery module with percentage display"

# 5. Push y crear PR
git push -u origin feat/waybar-battery-module
```

### Ejemplo de flujo INCORRECTO

```bash
# ❌ NO: commitear directo a main
git checkout main
git commit -m "update waybar"
git push

# ❌ NO: branch sin convención
git checkout -b mis-cambios

# ❌ NO: commit vago sin scope
git commit -m "updated files"

# ❌ NO: mezclar scopes
git commit -m "feat: add waybar module and refactor installer"

# ❌ NO: modificar submodules sin querer
git add .  # esto puede agregar cambios en submodules
```

---

## Submodules

Este repo usa git submodules para:

| Path | Repo | Qué es |
|---|---|---|
| `.config/nvim` | `MrUse77/Nvim-config` | Config de Neovim |
| `.zsh_plugins/fzf-tab` | `Aloxaf/fzf-tab` | Plugin zsh |
| `.zsh_plugins/zsh-autosuggestions` | `zsh-users/zsh-autosuggestions` | Plugin zsh |
| `.zsh_plugins/zsh-history-substring-search` | `zsh-users/zsh-history-substring-search` | Plugin zsh |
| `.zsh_plugins/zsh-syntax-highlighting` | `zsh-users/zsh-syntax-highlighting` | Plugin zsh |

### Reglas para submodules

1. **No modificar el contenido** de un submodule desde este repo.
2. **Para actualizar** un submodule:
   ```bash
   git submodule update --remote .zsh_plugins/fzf-tab
   git add .zsh_plugins/fzf-tab
   git commit -m "chore(zsh): update fzf-tab submodule to latest"
   ```
3. **Verificar siempre** que no se agregaron cambios accidentales:
   ```bash
   git diff --submodule
   ```
4. **Para agregar** un submodule nuevo, hacer un PR dedicado con justificación.

---

## Testing

### Config (raíz)

```bash
# Test de integración con Docker
bash test.sh
```

Esto construye un container Arch Linux, monta el repo, y corre el instalador.

### CLI (`cli/`)

```bash
cd cli

# Unit tests
go test ./...

# Linter
go vet ./...

# Build check
go build ./...

# Formateo
go fmt ./...
```

### Cuándo testear

| Cambié... | Debo correr... |
|---|---|
| Cualquier config en `.config/` | `bash test.sh` (si es posible) |
| Código Go en `cli/` | `go test ./...` + `go vet ./...` + `go build ./...` |
| `Dockerfile.test` o `test.sh` | `bash test.sh` |
| Solo docs | Nada (verificar que el markdown es válido) |

---

## Estilo y convenciones de código

### Archivos de configuración

- **Respetar el formato existente.** Si un archivo usa tabs, usá tabs. Si usa 2 espacios, usá 2 espacios.
- **Comentar cambios no obvios.** Un `# Why:` breve ayuda al futuro yo.
- **No dejar config comentada muerta.** Si no se usa, se borra.
- **Tema consistente:** Todo sigue la paleta TokyoNight. Verificar colores contra la paleta existente.

### Go (CLI)

- Seguir las convenciones estándar de Go (`gofmt`, `go vet`).
- Tests con `_test.go` en el mismo paquete.
- Documentar funciones públicas.
- Errores descriptivos con contexto: `fmt.Errorf("install: failed to copy %s: %w", path, err)`.

### Markdown

- Usar headings jerárquicos (`#`, `##`, `###`).
- Links relativos dentro del repo.
- Tablas para información estructurada.
- Code blocks con language hint (` ```bash `, ` ```go `, etc.).

---

## Resumen rápido

```
1. git checkout -b <tipo>/<descripcion>     ← branch con convención
2. hacer cambios mínimos                     ← solo lo necesario
3. git diff --submodule                      ← verificar submodules
4. tests si aplica                            ← go test / bash test.sh
5. git commit -m "<tipo>(<scope>): ..."      ← conventional commit
6. git push -u origin <branch>               ← push
7. abrir PR con template                      ← revisión
8. merge + delete branch                      ← limpieza
```

---

*Última actualización: julio 2025*
