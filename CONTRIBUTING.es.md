[English](CONTRIBUTING.md) | **Español**

# Cómo contribuir a HitStor

¡Gracias por su interés en mejorar HitStor! Los reportes de errores, las correcciones de documentación y los pull requests son bienvenidos.

## Antes de empezar

- Para errores e ideas de nuevas funciones, abra primero un issue para que podamos discutir el enfoque.
- Para problemas de seguridad, siga [SECURITY.es.md](SECURITY.es.md) en lugar de abrir un issue público.

## Desarrollo

Requisitos: Go 1.26+ (consulte `go.mod`). `protoc` solo es necesario si modifica archivos `.proto` (`make proto`).

```bash
make build   # compila los binarios en bin/
make test    # ejecuta el conjunto de pruebas
```

Consulte la [guía para desarrolladores](docs/dev-guide/index.md) para ver la descripción general del código, el laboratorio de pruebas y el proceso de publicación de versiones.

## Pull requests

- Mantenga los cambios acotados: una función o corrección por pull request.
- Agregue o actualice las pruebas. El proyecto sigue un enfoque de pruebas primero, con pruebas basadas en tablas.
- Ejecute `gofmt` y asegúrese de que `go vet ./...` y `go test ./...` pasen.
- Siga las convenciones del README (encapsulado de errores, `log/slog`, sin paquetes `utils/`).
- Use [Conventional Commits](https://www.conventionalcommits.org/) para los mensajes de commit (`feat:`, `fix:`, `docs:` ...).

## Licencia

Al contribuir, usted acepta que sus contribuciones se distribuyan bajo la
[GNU Affero General Public License v3.0](LICENSE), la misma licencia del proyecto.
