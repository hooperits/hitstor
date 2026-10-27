# Translations

HitStor's web dashboard and this documentation are available in **English** and **Spanish**. Both are built so that more languages can be added without code changes. The CLI, installers and log messages stay in English.

## Dashboard

### Choosing the language

The dashboard picks the language for each browser in this order:

1. The language chosen with the selector in the header (or on the login page), stored in the `hitstor_lang` cookie.
2. The server default, `serve_dashboard_language` in the server's `config.yaml` (for example `es`).
3. The browser's preferred languages (`Accept-Language`).
4. English.

```bash
hitstor config set serve_dashboard_language es
```

Restart the server after changing the default.

### How messages are stored

Messages are keyed by their **English text**. English needs no catalog, and any message without a translation is shown in English.

Each language is a directory under `internal/i18n/locales/`:

```
internal/i18n/locales/
├── en/
│   └── _meta.json
└── es/
    ├── _meta.json
    ├── common.json
    ├── login.json
    └── ...            one fragment per dashboard area
```

`_meta.json` names the language:

```json
{"code": "es", "name": "Español", "complete": true}
```

Fragments map English text to the translation. Placeholders in braces must be kept:

```json
{
  "Backups": "Respaldos",
  "{n} days ago": "hace {n} días"
}
```

In templates, text is marked with `{{t "Backups"}}` or `{{tf "{n} days ago" "n" .Days}}`. In JavaScript, with `t('Backups')` or `t('{n} days ago', {n: days})`.

### Adding a language

1. Create `internal/i18n/locales/<code>/_meta.json` with `"complete": false` (the code is the language tag browsers send, e.g. `fr` or `pt-BR`).
2. Copy the Spanish fragments as a starting point and translate the values, keeping the English keys and the `{placeholders}`.
3. Run the catalog tests:

    ```bash
    go test ./internal/i18n/ ./internal/dashboard/ -run 'TestCatalog|TestNegotiate'
    ```

    `TestCatalogsComplete` lists untranslated messages for languages marked `"complete": true` and, for every language, translations that are no longer used. `TestCatalogPlaceholders` checks the placeholders.

4. When every message is translated, set `"complete": true` so the tests keep the language complete as the dashboard changes.
5. Rebuild the server. The new language appears in the selector automatically.

### When you change dashboard text

Every new or changed English message must be added to each complete language; `TestCatalogsComplete` fails until it is. Messages shown from runtime values (for example job statuses translated with `{{t .Status}}`) are listed one per line in `internal/dashboard/i18n-dynamic/*.txt` so the test knows they are used.

## Documentation

The site uses [mkdocs-static-i18n](https://ultrabug.github.io/mkdocs-static-i18n/) with one file per language next to each page: `quickstart.md` (English) and `quickstart.es.md` (Spanish). A page without a translation falls back to English.

- Keep the English anchors on translated headings so links work in every language: `## Inicio rápido { #quick-start }`.
- Keep link targets pointing to the English file names (`quickstart.md#dashboard-password`); the plugin maps them to the right language.
- Menu titles for each language are set under `nav_translations` in `mkdocs.yml`.
- Terminology for Spanish is in the [Spanish glossary](i18n-glossary-es.md).

To add a documentation language, add it under the `i18n` plugin's `languages` in `mkdocs.yml` and create `<page>.<code>.md` files.

Build the site locally:

```bash
pip install -r requirements-docs.txt
make docs-build
```

## Repository files

`README.md`, `CONTRIBUTING.md`, `SECURITY.md` and `benchmarks/README.md` have Spanish versions (`*.es.md`) linked from the first line of each file.
