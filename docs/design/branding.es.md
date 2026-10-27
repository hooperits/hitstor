# Identidad visual de HitStor

Sistema de diseño para el dashboard web de HitStor y la experiencia de inicio de sesión. El objetivo es una identidad coherente y profesional: **profundidad azul marino + energía naranja + superficies claras y limpias**.

## Esencia de la marca { #brand-essence }

- **Confianza y seguridad**: fondos azul marino profundo, isotipo de candado, tipografía sobria
- **Acción y calidez**: acentos naranja en las acciones principales, la navegación activa y la sílaba `stor` del logotipo
- **Claridad**: áreas de contenido claras, tarjetas blancas, espaciado generoso

## Paleta de colores { #color-palette }

| Token | Hex / valor | Uso |
|-------|-------------|--------|
| `--brand-navy-deep` | `#071018` | Inicio del degradado |
| `--navy` | `#0b1f3a` | Punto medio del degradado, títulos sobre tarjetas claras |
| `--brand-navy-light` | `#132d4f` | Final del degradado |
| `--brand-text` | `#f4f7fb` | Texto sobre superficies azul marino |
| `--brand-text-muted` | `rgba(244,247,251,0.75)` | Texto secundario sobre azul marino |
| `--brand-accent` | `#ff7a45` | Sílaba `stor`, etiqueta de versión, barra de navegación activa |
| `--primary` | `#e8602c` | Botones, métricas, viñetas |
| `--primary-dark` | `#cf4f1f` | Botón al pasar el cursor (hover) |
| `--content-gradient` | `#f8fafc → #f1f5f9` | Panel derecho del inicio de sesión, fondo del contenido del dashboard |

### Degradados { #gradients }

**Superficie de marca azul marino** (encabezado, barra lateral, panel izquierdo del inicio de sesión):

```css
--brand-gradient: linear-gradient(145deg, #071018 0%, #0b1f3a 42%, #132d4f 100%);
```

**Capa de resplandor ambiental** (siempre combinada con superficies azul marino):

```css
--brand-glow: radial-gradient(circle at 20% 20%, rgba(232,96,44,0.18), transparent 42%),
              radial-gradient(circle at 80% 70%, rgba(255,255,255,0.06), transparent 35%);
```

## Sistema de logotipo { #logo-system }

### Isotipo (`assets/icon.png` → `internal/dashboard/static/icon.png`) { #isotipo-assetsiconpng-internaldashboardstaticiconpng }

- Marca de candado en un cuadrado redondeado sobre fondo oscuro
- Úselo solo en espacios compactos (encabezado, barra lateral, tarjeta de inicio de sesión)
- El radio del borde escala con el tamaño: 10px (sm), 12px (md), 18px (lg)
- Aplique siempre una sombra suave sobre fondos azul marino

### Logotipo (HTML, no una imagen) { #wordmark-html-not-an-image }

```html
<span class="brand-wordmark">
  <span class="wm-hit">hit</span><span class="wm-stor">stor</span>
</span>
```

| Sílaba | Color | Peso |
|--------|-------|------|
| `hit` | `--brand-text` (blanco sobre azul marino) | 800 |
| `stor` | `--brand-accent` (naranja) | 800 |

Espaciado entre letras: `-0.04em`. En la estructura del dashboard, prefiera el logotipo HTML en lugar de `logo.png` para una representación nítida con cualquier DPI.

### Cuándo usar cada recurso { #when-to-use-which-asset }

| Contexto | Recurso |
|---------|--------|
| Encabezado del dashboard | Isotipo + logotipo HTML (una vez; no se repite en la barra lateral) |
| Panel principal del inicio de sesión | Isotipo grande + logotipo grande |
| README / documentación externa | `logo.svg` / `logo-dark.svg` (logotipo SVG) |
| Favicon | `favicon.png` |

## Tipografía { #typography }

- **Interfaz**: `system-ui, -apple-system, "Segoe UI", Roboto, sans-serif`
- **Monoespaciada** (ID de repositorio, códigos): `SF Mono, Cascadia Code, Fira Code`
- **Tamaños del logotipo**: 2.75rem (panel principal del inicio de sesión), 1.35rem (barra lateral), 1.15rem (encabezado)

## Estructura de la interfaz (dashboard autenticado) { #layout-shell-authenticated-dashboard }

```
┌─────────────────────────────────────────────────────────┐
│  Navy header — isotipo + wordmark · repo id · version   │
├──────────┬──────────────────────────────────────────────┤
│  Navy    │  Light gradient content area                 │
│  sidebar │  White cards, orange metric accents          │
│  (nav)   │                                              │
└──────────┴──────────────────────────────────────────────┘
```

- **Encabezado**: degradado azul marino de ancho completo con resplandor; fijo en la parte superior; **una sola** ubicación de la marca (isotipo + logotipo)
- **Barra lateral**: el mismo degradado azul marino, solo navegación; el elemento activo tiene un borde izquierdo naranja
- **Contenido**: fondo `--content-gradient`; las tarjetas se mantienen blancas con un acento naranja a la izquierda

## Componentes { #components }

### `.brand-surface`

Degradado azul marino + capa de resplandor. Se usa en el panel izquierdo del inicio de sesión y en el encabezado del dashboard.

### `.brand-icon` (+ `-sm` / `-md` / `-lg`)

Tamaños predefinidos del isotipo con radio y sombra uniformes.

### `.brand-wordmark`

Tipografía del logotipo; combínela con modificadores de tamaño (`brand-wordmark-header`, `login-brand-wordmark`).

### Tarjeta de inicio de sesión { #login-card }

Tarjeta blanca sobre degradado claro: borde `var(--border)`, radio `calc(var(--radius) + 4px)`, sombra `var(--shadow-hover)`.

### Botón principal { #primary-button }

Fondo `--primary`, texto blanco, ancho completo en el inicio de sesión; al pasar el cursor, `--primary-dark`.

## Implementación en CSS { #css-implementation }

Todos los tokens están en `internal/dashboard/static/style.css`, dentro de `:root`. Las clases compartidas llevan el prefijo `brand-` y se reutilizan en:

- `internal/dashboard/templates/login.html`
- `internal/dashboard/templates/layout.html`

## Recursos y regeneración { #assets-regeneration }

| Archivo | Función |
|------|------|
| `assets/icon.png` | Isotipo maestro |
| `assets/logo.svg` | Logotipo para fondos claros (externo) |
| `assets/logo-dark.svg` | Logotipo para fondos oscuros (externo) |
| `scripts/render-logos.sh` | Regenera las exportaciones PNG mediante resvg |

Después de modificar los recursos estáticos, vuelva a compilar el binario del servidor (`make build`) para que `go:embed` incluya los archivos nuevos.

## Control de calidad visual { #visual-qa }

```bash
# Resumen (Overview) después del inicio de sesión + todas las páginas
./scripts/screenshot-dashboard.sh

# Comparación de la identidad visual antes/después
OUTFILE=tmp/screenshots/brand-audit/overview-after.png \
  node scripts/.screenshot-runner/capture-overview.mjs
```

Las capturas de pantalla se guardan en `tmp/screenshots/` (excluido mediante gitignore).
