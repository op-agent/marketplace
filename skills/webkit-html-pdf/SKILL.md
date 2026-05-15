---
id: skill-webkit-html-pdf
name: WebKit HTML PDF
description: Export local HTML files or localhost pages to stable A4 PDFs on macOS by rendering them in WebKit first and paginating the result. Use this when browser Print to PDF drops text, breaks Chinese rendering, or fails to preserve the on-screen layout.
---

# WebKit HTML PDF

Use this skill when the user wants a local HTML file, `file://` page, or `http://127.0.0.1` / `http://localhost` page exported to PDF with the same visual rendering they see in the browser.

This skill exists because browser headless print can fail on Chinese text or complex CSS. The bundled workflow is:

1. Render the page in macOS WebKit with `WKWebView`.
2. Export one long PDF from the actual rendered content height.
3. Split that result into A4 pages with PyMuPDF.

## Requirements

- macOS
- `swift`
- `python3`
- Python package `pymupdf` (`import fitz`)

If `fitz` is missing, install it with:

```bash
python3 -m pip install --user pymupdf
```

## Primary Command

Run the bundled CLI from this skill directory:

```bash
python3 scripts/export_html_pdf.py <input-html-or-url> <output-pdf>
```

Examples:

```bash
python3 scripts/export_html_pdf.py ~/note/raw/opagent/熵潮科技.html ~/note/raw/opagent/熵潮科技.pdf
```

```bash
python3 scripts/export_html_pdf.py http://127.0.0.1:3000 /tmp/site.pdf
```

## Useful Flags

- `--mode a4|long`: default `a4`. `long` keeps the single long page result.
- `--long-output <path>`: keep the intermediate long-page PDF as a second file.
- `--wait-ms <n>`: wait after page load before measuring/exporting. Increase for JS-heavy pages.
- `--width <px>`: viewport width for WebKit rendering. Default `1440`.
- `--hide-selector <css>`: hide fixed toolbars or UI controls before export. Repeatable.

Example:

```bash
python3 scripts/export_html_pdf.py \
  ~/note/raw/opagent/熵潮科技.html \
  ~/note/raw/opagent/熵潮科技.pdf \
  --long-output ~/note/raw/opagent/熵潮科技.long.pdf \
  --hide-selector '.screen-toolbar'
```

## Rules

- Prefer the bundled script instead of re-deriving a new export flow.
- For local files, pass the path directly; the script converts it to `file://`.
- For localhost pages, ensure the page is already reachable before exporting.
- If the output has unexpected blank pages, keep the long PDF with `--long-output` and inspect whether the page itself contains trailing empty space.
- Do not use this skill on non-macOS hosts unless the user explicitly wants a fallback workflow.
