#!/usr/bin/env bash
# Regenerate PNG logos from SVG sources. Requires: npm + @resvg/resvg-js
set -euo pipefail
cd "$(dirname "$0")/.."
npm install --no-save @resvg/resvg-js >/dev/null
node <<'NODE'
const { Resvg } = require('@resvg/resvg-js');
const fs = require('fs');
// The wordmark is SVG <text> in Segoe UI; use the Windows fonts when rendering from WSL.
const fontDirs = ['/mnt/c/Windows/Fonts', 'C:/Windows/Fonts'].filter((d) => fs.existsSync(d));
function render(svg, out, w) {
  const resvg = new Resvg(fs.readFileSync(svg), { fitTo: { mode: 'width', value: w }, font: { loadSystemFonts: true, fontDirs } });
  fs.writeFileSync(out, resvg.render().asPng());
  console.log('wrote', out);
}
render('assets/icon.svg', 'assets/icon.png', 1024);
render('assets/logo.svg', 'assets/logo.png', 800);
render('assets/logo-dark.svg', 'assets/logo-dark.png', 800);
render('assets/favicon.svg', 'assets/favicon.png', 64);
render('assets/icon.svg', 'assets/apple-touch-icon.png', 180);
NODE
cp assets/icon.svg assets/icon.png assets/favicon.svg assets/favicon.png assets/logo.png internal/dashboard/static/
cp assets/logo.png assets/logo-dark.png assets/favicon.png docs/assets/
sh "$(dirname "$0")/render-windows-icon.sh"
echo "Logo assets synced."
