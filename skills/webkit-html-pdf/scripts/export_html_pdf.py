#!/usr/bin/env python3
from __future__ import annotations

import argparse
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path
from urllib.parse import urlparse

try:
    import fitz
except ImportError as exc:  # pragma: no cover - environment specific
    raise SystemExit(
        "Missing dependency: pymupdf. Install with `python3 -m pip install --user pymupdf`."
    ) from exc


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description=(
            "Render a local HTML file or URL in macOS WebKit and export it to PDF. "
            "Default mode paginates the result into A4 pages."
        )
    )
    parser.add_argument("input", help="Local HTML path or absolute URL")
    parser.add_argument("output", help="Output PDF path")
    parser.add_argument(
        "--mode",
        choices=("a4", "long"),
        default="a4",
        help="Export as A4 pages or keep a single long page",
    )
    parser.add_argument(
        "--page-size",
        default="a4",
        help="Target page size for paginated mode. Default: a4",
    )
    parser.add_argument(
        "--width",
        type=int,
        default=1440,
        help="Viewport width in CSS pixels for WebKit rendering. Default: 1440",
    )
    parser.add_argument(
        "--wait-ms",
        type=int,
        default=1000,
        help="Extra wait time after load before measuring/export. Default: 1000",
    )
    parser.add_argument(
        "--hide-selector",
        action="append",
        default=[],
        help="CSS selector to hide before export. Repeat for multiple selectors.",
    )
    parser.add_argument(
        "--long-output",
        help="Optional path to also keep the intermediate long-page PDF",
    )
    return parser.parse_args()


def ensure_macos() -> None:
    if sys.platform != "darwin":
        raise SystemExit("This script requires macOS because it uses WKWebView via Swift.")


def to_input_url(raw: str) -> str:
    parsed = urlparse(raw)
    if parsed.scheme in {"http", "https", "file"}:
        return raw
    path = Path(raw).expanduser().resolve()
    if not path.exists():
        raise SystemExit(f"Input file does not exist: {path}")
    return path.as_uri()


def paginate_pdf(long_pdf: Path, output_pdf: Path, page_size: str) -> None:
    source = fitz.open(str(long_pdf))
    if source.page_count == 0:
        raise SystemExit(f"Rendered PDF has no pages: {long_pdf}")
    target = fitz.open()
    page_width, page_height = fitz.paper_size(page_size.lower())
    for page_index in range(source.page_count):
        page = source[page_index]
        rect = page.rect
        if rect.width <= 0 or rect.height <= 0:
            continue
        scale = page_width / rect.width
        slice_height = page_height / scale
        start_y = 0.0
        while start_y < rect.height - 0.5:
            end_y = min(start_y + slice_height, rect.height)
            clip = fitz.Rect(0, start_y, rect.width, end_y)
            new_page = target.new_page(width=page_width, height=page_height)
            rendered_height = (end_y - start_y) * scale
            dest = fitz.Rect(0, 0, page_width, rendered_height)
            new_page.show_pdf_page(dest, source, page_index, clip=clip)
            start_y = end_y
    output_pdf.parent.mkdir(parents=True, exist_ok=True)
    target.save(str(output_pdf))


def run_webkit_render(
    input_url: str,
    long_pdf: Path,
    width: int,
    wait_ms: int,
    hide_selectors: list[str],
) -> None:
    skill_dir = Path(__file__).resolve().parent
    swift_script = skill_dir / "render_webkit_pdf.swift"
    cmd = [
        "swift",
        str(swift_script),
        "--input",
        input_url,
        "--output",
        str(long_pdf),
        "--width",
        str(width),
        "--wait-ms",
        str(wait_ms),
    ]
    selectors = [".screen-toolbar", "[data-print-hide='true']"] + hide_selectors
    for selector in selectors:
        cmd.extend(["--hide-selector", selector])
    subprocess.run(cmd, check=True)


def main() -> int:
    args = parse_args()
    ensure_macos()
    input_url = to_input_url(args.input)
    output_pdf = Path(args.output).expanduser().resolve()
    long_output = Path(args.long_output).expanduser().resolve() if args.long_output else None

    with tempfile.TemporaryDirectory(prefix="webkit-html-pdf-") as tmpdir:
        long_pdf = Path(tmpdir) / "rendered.long.pdf"
        run_webkit_render(
            input_url=input_url,
            long_pdf=long_pdf,
            width=args.width,
            wait_ms=args.wait_ms,
            hide_selectors=args.hide_selector,
        )
        if long_output is not None:
            long_output.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(long_pdf, long_output)
        if args.mode == "long":
            output_pdf.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(long_pdf, output_pdf)
        else:
            paginate_pdf(long_pdf, output_pdf, args.page_size)
    print(output_pdf)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
