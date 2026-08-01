# Versionado y releases

## Esquema: SemVer

Las versiones siguen [Semantic Versioning 2.0.0](https://semver.org/):

```text
vMAJOR.MINOR.PATCH
```

- **MAJOR**: cambios incompatibles (`feat!`, breaking change).
- **MINOR**: funcionalidad nueva (`feat`).
- **PATCH**: correcciones (`fix`).

Mientras el proyecto está en `0.x`, los cambios de MINOR pueden ser
incompatibles sin subir MAJOR. `v1.0.0` es el primer release estable.

`chore`, `docs`, `test` y `refactor` no generan release por sí solos: se
taggea cuando hay un bump real.

## Qué se versiona

El tag aplica al repo completo, y **la versión del CLI (`moonarch-cli`)
coincide con el tag del repo**. La versión se inyecta en el build con
`-ldflags "-X github.com/MrUse77/dots-cli/cmd.Version=<tag>"`. Sin tag
(el build local), el CLI reporta `dev`.

## Proceso de release

1. Todo el trabajo entra a `main` por PR (conventional commits).
2. Crear el tag en `main`:

   ```bash
   git tag v0.1.0 && git push origin v0.1.0
   ```

3. El workflow `.github/workflows/release.yml` (on-tag `v*`) hace el
   resto:
   - Valida que el tag cumpla `vMAJOR.MINOR.PATCH`.
   - Compila `moonarch-cli` para `linux/amd64` y `linux/arm64` con
     `-trimpath` y la versión inyectada.
   - Genera el changelog desde los commits `feat`/`fix` del rango
     (desde el tag anterior; el primer release usa el historial completo).
   - Crea el GitHub Release con los binarios attachados.

Los binarios del release son la fuente para el instalador sin Go y para
el futuro `moonarch-cli update`.

## Verificación

```bash
./moonarch-cli version   # dev en builds locales
# en un release: moonarch v0.1.0 (linux/amd64)
```
