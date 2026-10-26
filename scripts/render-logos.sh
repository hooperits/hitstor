#!/usr/bin/env bash
# Regenerate PNG logos from SVG sources. Requires: npm + @resvg/resvg-js
set -euo pipefail
cd "$(dirname "$0")/.."
npm install --no-save @resvg/resvg-js >/dev/null
node <<'NODE'
const { Resvg } = require('@resvg/resvg-js');
const fs = require('fs');
function render(svg, out, w) {
  const resvg = new Resvg(fs.readFileSync(svg), { fitTo: { mode: 'width', value: w } });
  fs.writeFileSync(out, resvg.render().asPng());
  console.log('wrote', out);
}
render('assets/logo.svg', 'assets/logo.png', 800);
render('assets/logo-dark.svg', 'assets/logo-dark.png', 800);
render('assets/favicon.svg', 'assets/favicon.png', 64);
render('assets/favicon.svg', 'assets/apple-touch-icon.png', 180);
NODE
cp assets/logo.png internal/dashboard/static/logo.png
cp assets/favicon.png internal/dashboard/static/favicon.png
cp assets/logo.png docs/assets/logo.png
cp assets/favicon.png docs/assets/favicon.png
sh "$(dirname "$0")/render-windows-icon.sh"
echo "Logo assets synced."