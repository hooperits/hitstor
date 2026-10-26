# HitStor Visual Branding

Design system for the HitStor web dashboard and login experience. The goal is a cohesive, professional identity: **navy depth + orange energy + clean light surfaces**.

## Brand essence

- **Trust & security** — deep navy backgrounds, padlock isotipo, restrained typography
- **Action & warmth** — orange accents on primary actions, active nav, and the `stor` wordmark syllable
- **Clarity** — light content areas, white cards, generous spacing

## Color palette

| Token | Hex / value | Usage |
|-------|-------------|--------|
| `--brand-navy-deep` | `#071018` | Gradient start |
| `--navy` | `#0b1f3a` | Gradient mid, headings on light cards |
| `--brand-navy-light` | `#132d4f` | Gradient end |
| `--brand-text` | `#f4f7fb` | Text on navy surfaces |
| `--brand-text-muted` | `rgba(244,247,251,0.75)` | Secondary text on navy |
| `--brand-accent` | `#ff7a45` | `stor` syllable, version pill, nav active bar |
| `--primary` | `#e8602c` | Buttons, metrics, bullet dots |
| `--primary-dark` | `#cf4f1f` | Button hover |
| `--content-gradient` | `#f8fafc → #f1f5f9` | Login right panel, dashboard content background |

### Gradients

**Navy brand surface** (header, sidebar, login left panel):

```css
--brand-gradient: linear-gradient(145deg, #071018 0%, #0b1f3a 42%, #132d4f 100%);
```

**Ambient glow overlay** (always paired with navy surfaces):

```css
--brand-glow: radial-gradient(circle at 20% 20%, rgba(232,96,44,0.18), transparent 42%),
              radial-gradient(circle at 80% 70%, rgba(255,255,255,0.06), transparent 35%);
```

## Logo system

### Isotipo (`assets/icon.png` → `internal/dashboard/static/icon.png`)

- Rounded square padlock mark on dark ground
- Use alone in compact spaces (header, sidebar, login card)
- Border radius scales with size: 10px (sm), 12px (md), 18px (lg)
- Always apply soft drop shadow on navy backgrounds

### Wordmark (HTML, not an image)

```html
<span class="brand-wordmark">
  <span class="wm-hit">hit</span><span class="wm-stor">stor</span>
</span>
```

| Syllable | Color | Weight |
|----------|-------|--------|
| `hit` | `--brand-text` (white on navy) | 800 |
| `stor` | `--brand-accent` (orange) | 800 |

Letter-spacing: `-0.04em`. Prefer the HTML wordmark over `logo.png` in the dashboard shell for crisp rendering at any DPI.

### When to use which asset

| Context | Asset |
|---------|--------|
| Dashboard header | Isotipo + HTML wordmark (once; not repeated in sidebar) |
| Login hero panel | Large isotipo + large wordmark |
| README / external docs | `logo.svg` / `logo-dark.svg` (SVG wordmark) |
| Favicon | `favicon.png` |

## Typography

- **UI**: `system-ui, -apple-system, "Segoe UI", Roboto, sans-serif`
- **Monospace** (repo IDs, codes): `SF Mono, Cascadia Code, Fira Code`
- **Wordmark sizes**: 2.75rem (login hero), 1.35rem (sidebar), 1.15rem (header)

## Layout shell (authenticated dashboard)

```
┌─────────────────────────────────────────────────────────┐
│  Navy header — isotipo + wordmark · repo id · version   │
├──────────┬──────────────────────────────────────────────┤
│  Navy    │  Light gradient content area                 │
│  sidebar │  White cards, orange metric accents          │
│  (nav)   │                                              │
└──────────┴──────────────────────────────────────────────┘
```

- **Header**: full-width navy gradient with glow; sticky top; **single** brand placement (isotipo + wordmark)
- **Sidebar**: same navy gradient, navigation only; active item has orange left border
- **Content**: `--content-gradient` background; cards stay white with orange left accent

## Components

### `.brand-surface`

Navy gradient + glow overlay. Used on login left panel and dashboard header.

### `.brand-icon` (+ `-sm` / `-md` / `-lg`)

Isotipo sizing presets with consistent radius and shadow.

### `.brand-wordmark`

Wordmark typography; combine with size modifiers (`brand-wordmark-header`, `login-brand-wordmark`).

### Login card

White card on light gradient: border `var(--border)`, radius `calc(var(--radius) + 4px)`, shadow `var(--shadow-hover)`.

### Primary button

`--primary` background, white text, full-width on login; hover `--primary-dark`.

## CSS implementation

All tokens live in `internal/dashboard/static/style.css` under `:root`. Shared classes are prefixed `brand-` and reused by:

- `internal/dashboard/templates/login.html`
- `internal/dashboard/templates/layout.html`

## Assets & regeneration

| File | Role |
|------|------|
| `assets/icon.png` | Master isotipo |
| `assets/logo.svg` | Light-background wordmark (external) |
| `assets/logo-dark.svg` | Dark-background wordmark (external) |
| `scripts/render-logos.sh` | Regenerate PNG exports via resvg |

After changing static assets, rebuild the server binary (`make build`) so `go:embed` picks up new files.

## Visual QA

```bash
# Post-login Overview + all pages
./scripts/screenshot-dashboard.sh

# Before/after branding comparison
OUTFILE=tmp/screenshots/brand-audit/overview-after.png \
  node scripts/.screenshot-runner/capture-overview.mjs
```

Screenshots are written to `tmp/screenshots/` (gitignored).