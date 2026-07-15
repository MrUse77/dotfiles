## Qué cambia

Breve descripción de qué se modificó y por qué.

## Archivos afectados

- `path/al/archivo` — descripción del cambio

## Testing

- [ ] `bash test.sh` pasa (si aplica)
- [ ] `cd cli && go test ./...` pasa (si hay cambios en cli/)
- [ ] `cd cli && go vet ./...` sin warnings
- [ ] Probado en un entorno real / Docker

## Checklist

- [ ] La branch sigue la convención de nombres (`<tipo>/<descripcion-kebab-case>`)
- [ ] Los commits siguen Conventional Commits (`tipo(scope): descripción`)
- [ ] No se mezclan cambios de config con cambios de CLI sin justificación
- [ ] No se modificaron submodules sin intención (verificar con `git diff --submodule`)
- [ ] No se agregaron archivos binarios innecesarios
- [ ] Los archivos de config son válidos (JSON, TOML, CSS sin errores de sintaxis)
