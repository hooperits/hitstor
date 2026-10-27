# Traducciones

El dashboard web de HitStor y esta documentación están disponibles en **inglés** y **español**. Ambos están preparados para agregar más idiomas sin cambiar código. La CLI, los instaladores y los mensajes de registro se mantienen en inglés.

## Dashboard { #dashboard }

### Elección del idioma { #choosing-the-language }

El dashboard elige el idioma de cada navegador en este orden:

1. El idioma elegido con el selector de la barra superior (o de la página de inicio de sesión), guardado en la cookie `hitstor_lang`.
2. El idioma por defecto del servidor, `serve_dashboard_language` en el `config.yaml` del servidor (por ejemplo `es`).
3. Los idiomas preferidos del navegador (`Accept-Language`).
4. Inglés.

```bash
hitstor config set serve_dashboard_language es
```

Reinicie el servidor después de cambiar el idioma por defecto.

### Cómo se guardan los mensajes { #how-messages-are-stored }

Los mensajes se identifican por su **texto en inglés**. El inglés no necesita catálogo, y cualquier mensaje sin traducción se muestra en inglés.

Cada idioma es un directorio dentro de `internal/i18n/locales/`:

```
internal/i18n/locales/
├── en/
│   └── _meta.json
└── es/
    ├── _meta.json
    ├── common.json
    ├── login.json
    └── ...            un fragmento por área del dashboard
```

`_meta.json` identifica el idioma:

```json
{"code": "es", "name": "Español", "complete": true}
```

Los fragmentos asocian el texto en inglés con su traducción. Los marcadores entre llaves deben conservarse:

```json
{
  "Backups": "Respaldos",
  "{n} days ago": "hace {n} días"
}
```

En las plantillas, el texto se marca con `{{t "Backups"}}` o `{{tf "{n} days ago" "n" .Days}}`. En JavaScript, con `t('Backups')` o `t('{n} days ago', {n: days})`.

### Agregar un idioma { #adding-a-language }

1. Cree `internal/i18n/locales/<código>/_meta.json` con `"complete": false` (el código es la etiqueta de idioma que envían los navegadores, por ejemplo `fr` o `pt-BR`).
2. Copie los fragmentos en español como punto de partida y traduzca los valores, conservando las claves en inglés y los `{marcadores}`.
3. Ejecute las pruebas del catálogo:

    ```bash
    go test ./internal/i18n/ ./internal/dashboard/ -run 'TestCatalog|TestNegotiate'
    ```

    `TestCatalogsComplete` lista los mensajes sin traducir de los idiomas marcados con `"complete": true` y, para todos los idiomas, las traducciones que ya no se usan. `TestCatalogPlaceholders` revisa los marcadores.

4. Cuando todos los mensajes estén traducidos, cambie a `"complete": true` para que las pruebas mantengan el idioma completo a medida que cambie el dashboard.
5. Vuelva a compilar el servidor. El nuevo idioma aparece automáticamente en el selector.

### Al cambiar textos del dashboard { #when-you-change-dashboard-text }

Cada mensaje en inglés nuevo o modificado debe agregarse a todos los idiomas completos; `TestCatalogsComplete` falla hasta que se haga. Los mensajes que se muestran a partir de valores en tiempo de ejecución (por ejemplo, estados de tareas traducidos con `{{t .Status}}`) se listan, uno por línea, en `internal/dashboard/i18n-dynamic/*.txt` para que la prueba sepa que se usan.

## Documentación { #documentation }

El sitio usa [mkdocs-static-i18n](https://ultrabug.github.io/mkdocs-static-i18n/) con un archivo por idioma junto a cada página: `quickstart.md` (inglés) y `quickstart.es.md` (español). Una página sin traducción se muestra en inglés.

- Conserve los anchors en inglés en los encabezados traducidos para que los enlaces funcionen en todos los idiomas: `## Inicio rápido { #quick-start }`.
- Mantenga los destinos de los enlaces apuntando a los nombres de archivo en inglés (`quickstart.md#dashboard-password`); el plugin los dirige al idioma correcto.
- Los títulos del menú de cada idioma se definen en `nav_translations` dentro de `mkdocs.yml`.
- La terminología en español está en el [glosario de español](i18n-glossary-es.md).

Para agregar un idioma a la documentación, agréguelo en `languages` del plugin `i18n` en `mkdocs.yml` y cree los archivos `<página>.<código>.md`.

Compile el sitio localmente:

```bash
pip install -r requirements-docs.txt
make docs-build
```

## Archivos del repositorio { #repository-files }

`README.md`, `CONTRIBUTING.md`, `SECURITY.md` y `benchmarks/README.md` tienen versiones en español (`*.es.md`) enlazadas desde la primera línea de cada archivo.
