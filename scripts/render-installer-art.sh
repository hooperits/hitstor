#!/usr/bin/env bash
# Regenerate the NSIS installer artwork from the SVG brand sources:
#   packaging/nsis/header.bmp   150x57  page header (white, horizontal lockup)
#   packaging/nsis/welcome.bmp  164x314 welcome/finish sidebar (navy, stacked lockup)
# Requires: npm + @resvg/resvg-js, python3 + Pillow.
# Art is rendered at 4x and downsampled for clean edges.
set -euo pipefail
cd "$(dirname "$0")/.."
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
npm install --no-save @resvg/resvg-js >/dev/null

# The isotipo, without its outer <svg> wrapper, for embedding.
ICON_BODY="$(sed -e '1d' -e '$d' assets/icon.svg)"

cat >"$TMP/welcome.svg" <<SVG
<svg xmlns="http://www.w3.org/2000/svg" width="656" height="1256" viewBox="0 0 164 314">
  <defs>
    <linearGradient id="bg" x1="0" y1="0" x2="0.6" y2="1">
      <stop offset="0" stop-color="#071018"/>
      <stop offset="0.42" stop-color="#0B1F3A"/>
      <stop offset="1" stop-color="#132D4F"/>
    </linearGradient>
    <radialGradient id="glow" cx="0.2" cy="0.18" r="0.55">
      <stop offset="0" stop-color="#E8602C" stop-opacity="0.22"/>
      <stop offset="1" stop-color="#E8602C" stop-opacity="0"/>
    </radialGradient>
    <linearGradient id="stor" x1="0" y1="0" x2="1" y2="0">
      <stop offset="0" stop-color="#FF7A45"/>
      <stop offset="1" stop-color="#FFB08A"/>
    </linearGradient>
  </defs>
  <rect width="164" height="314" fill="url(#bg)"/>
  <rect width="164" height="314" fill="url(#glow)"/>
  <svg x="46" y="74" width="72" height="72" viewBox="0 0 64 64">
$ICON_BODY
  </svg>
  <text x="82" y="190" text-anchor="middle" font-family="Segoe UI, sans-serif" font-size="34" font-weight="800" letter-spacing="-1.3"><tspan fill="#F4F7FB">hit</tspan><tspan fill="url(#stor)">stor</tspan></text>
  <text x="82" y="212" text-anchor="middle" font-family="Segoe UI, sans-serif" font-size="9.5" fill="#F4F7FB" fill-opacity="0.75">Encrypted, deduplicated backups</text>
  <rect x="62" y="284" width="40" height="2.5" rx="1.25" fill="#FF7A45" fill-opacity="0.8"/>
</svg>
SVG

cat >"$TMP/header.svg" <<SVG
<svg xmlns="http://www.w3.org/2000/svg" width="600" height="228" viewBox="0 0 150 57">
  <rect width="150" height="57" fill="#FFFFFF"/>
  <svg x="6" y="6.5" width="137" height="44" viewBox="18 26 432 108" preserveAspectRatio="xMidYMid meet">
$(sed -e '1d' -e '$d' assets/logo.svg)
  </svg>
</svg>
SVG

node - "$TMP" <<'NODE'
const { Resvg } = require('@resvg/resvg-js');
const fs = require('fs');
const dir = process.argv[2];
const fontDirs = ['/mnt/c/Windows/Fonts', 'C:/Windows/Fonts'].filter((d) => fs.existsSync(d));
for (const n of ['welcome', 'header']) {
  const r = new Resvg(fs.readFileSync(`${dir}/${n}.svg`), { font: { loadSystemFonts: true, fontDirs } });
  fs.writeFileSync(`${dir}/${n}.png`, r.render().asPng());
}
NODE

python3 - "$TMP" <<'PY'
import sys
from PIL import Image
d = sys.argv[1]
for name, size in (("welcome", (164, 314)), ("header", (150, 57))):
    img = Image.open(f"{d}/{name}.png").convert("RGBA")
    flat = Image.new("RGB", img.size, (255, 255, 255))
    flat.paste(img, mask=img.split()[3])
    flat.resize(size, Image.LANCZOS).save(f"packaging/nsis/{name}.bmp", format="BMP")
    print(f"wrote packaging/nsis/{name}.bmp {size[0]}x{size[1]}")
PY
