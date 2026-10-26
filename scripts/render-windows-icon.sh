#!/usr/bin/env bash
# Generate packaging/nsis/hitstor.ico from assets/icon.png for Windows ARP / installer UI.
set -euo pipefail
cd "$(dirname "$0")/.."
python3 <<'PY'
from pathlib import Path
from PIL import Image

src = Path("assets/icon.png")
out = Path("packaging/nsis/hitstor.ico")
img = Image.open(src).convert("RGBA")
sizes = [256, 128, 64, 48, 32, 16]
img.save(out, format="ICO", sizes=[(s, s) for s in sizes])
print(f"wrote {out} ({', '.join(str(s) for s in sizes)}px)")
PY