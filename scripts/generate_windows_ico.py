#!/usr/bin/env python3

import argparse
from pathlib import Path

from PIL import Image


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Generate a Windows .ico file with BMP-backed icon sizes.",
    )
    parser.add_argument("--input", required=True, help="Source PNG path")
    parser.add_argument("--output", required=True, help="Destination ICO path")
    args = parser.parse_args()

    src = Path(args.input)
    dst = Path(args.output)

    img = Image.open(src).convert("RGBA")

    # Keep the set intentionally shell-friendly. BMP-backed subimages are
    # more reliable for Windows taskbar/explorer than PNG-only ICO entries.
    sizes = [(16, 16), (24, 24), (32, 32), (48, 48), (64, 64), (128, 128)]

    dst.parent.mkdir(parents=True, exist_ok=True)
    img.save(dst, format="ICO", sizes=sizes, bitmap_format="bmp")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
