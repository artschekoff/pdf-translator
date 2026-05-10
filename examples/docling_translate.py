#!/usr/bin/env python3
"""Translate a PDF using Docling for extraction + OpenAI for translation."""

import os
import re
import sys
import shutil
import subprocess
import argparse
from pathlib import Path

MDTOPDF_SCRIPT = Path.home() / ".scripts/pdf/mdtopdf.sh"

from docling.datamodel.base_models import InputFormat
from docling.datamodel.pipeline_options import PdfPipelineOptions
from docling.document_converter import DocumentConverter, PdfFormatOption
from docling_core.types.doc.document import ImageRefMode
from openai import OpenAI

client = OpenAI(api_key=os.environ["OPENAI_API_KEY"])

BATCH_SIZE = 4000  # chars per API call (excluding base64 image data)

# Matches markdown image tags including base64 data URIs
_IMG_RE = re.compile(r"!\[[^\]]*\]\([^)]+\)")
_HEADING_RE = re.compile(r"^#{1,6}\s+")


def translate_batch(texts: list[str], target_lang: str) -> list[str]:
    sep = "\n\n<<<SEP>>>\n\n"
    combined = sep.join(texts)
    resp = client.chat.completions.create(
        model="gpt-4o-mini",
        messages=[
            {
                "role": "system",
                "content": (
                    f"You are a professional translator. Translate the following Markdown text to {target_lang}. "
                    f"The text contains multiple segments separated by <<<SEP>>>. "
                    "Rules:\n"
                    "- Preserve ALL <<<SEP>>> separators exactly as-is.\n"
                    "- Preserve Markdown syntax: #, ##, **, *, -, |, ` etc.\n"
                    "- Translate heading text but keep the # prefix.\n"
                    "- Translate table cell content but keep | structure.\n"
                    "- Do not translate code blocks (``` ... ```).\n"
                    "- Do not modify image placeholders like <!-- IMG_0 -->.\n"
                    "- Output only the translated Markdown with separators intact."
                ),
            },
            {"role": "user", "content": combined},
        ],
    )
    result = resp.choices[0].message.content.strip()
    parts = result.split("<<<SEP>>>")
    while len(parts) < len(texts):
        parts.append("")
    return [p.strip() for p in parts[: len(texts)]]


def batch_chunks(chunks: list[str], max_chars: int) -> list[list[str]]:
    batches, current, size = [], [], 0
    for chunk in chunks:
        if current and size + len(chunk) > max_chars:
            batches.append(current)
            current, size = [], 0
        current.append(chunk)
        size += len(chunk)
    if current:
        batches.append(current)
    return batches


def split_markdown_sections(md: str) -> list[str]:
    sections = []
    current: list[str] = []
    for line in md.splitlines(keepends=True):
        if _HEADING_RE.match(line) and current:
            sections.append("".join(current))
            current = []
        current.append(line)
    if current:
        sections.append("".join(current))
    return sections if sections else [md]


def extract_images(md: str) -> tuple[str, dict[str, str]]:
    """Replace image tags with placeholders, return modified md + image map."""
    images: dict[str, str] = {}
    counter = 0

    def replacer(m: re.Match) -> str:
        nonlocal counter
        key = f"<!-- IMG_{counter} -->"
        images[key] = m.group(0)
        counter += 1
        return key

    return _IMG_RE.sub(replacer, md), images


def restore_images(md: str, images: dict[str, str]) -> str:
    for key, tag in images.items():
        md = md.replace(key, tag)
    return md


def main():
    parser = argparse.ArgumentParser(description="Translate PDF via Docling + OpenAI")
    parser.add_argument("input", help="Input PDF path")
    parser.add_argument("--to", default="Russian", help="Target language (default: Russian)")
    parser.add_argument("--output", help="Output markdown file (default: <input>_translated.md)")
    parser.add_argument("--batch-size", type=int, default=BATCH_SIZE, help="Max chars per API call")
    parser.add_argument("--no-images", action="store_true", help="Skip image extraction")
    parser.add_argument("--no-pdf", action="store_true", help="Skip PDF generation")
    parser.add_argument("--dark", action="store_true", help="Use dark theme for PDF")
    args = parser.parse_args()

    input_path = Path(args.input)
    if not input_path.exists():
        print(f"Error: {input_path} not found", file=sys.stderr)
        sys.exit(1)

    output_path = Path(args.output) if args.output else input_path.with_name(
        input_path.stem + f"_translated_{args.to.lower()}.md"
    )
    images_dir = output_path.parent / (output_path.stem + "_images")

    print(f"Converting {input_path} with Docling...")
    pipeline_options = PdfPipelineOptions(
        generate_picture_images=not args.no_images,
        images_scale=2.0,
    )
    converter = DocumentConverter(
        format_options={InputFormat.PDF: PdfFormatOption(pipeline_options=pipeline_options)}
    )
    result = converter.convert(str(input_path))
    doc = result.document

    print("Exporting to Markdown...")
    if args.no_images:
        markdown = doc.export_to_markdown(image_mode=ImageRefMode.PLACEHOLDER)
    else:
        images_dir.mkdir(parents=True, exist_ok=True)
        # save_as_markdown writes images to artifact_path and uses relative refs
        doc.save_as_markdown(
            output_path,
            image_mode=ImageRefMode.REFERENCED,
            artifacts_dir=images_dir,
        )
        markdown = output_path.read_text(encoding="utf-8")
        # Docling writes absolute paths — rewrite to relative so PDF renderer works
        markdown = markdown.replace(str(images_dir) + "/", images_dir.name + "/")

    print("Extracting images from Markdown...")
    markdown_no_imgs, images = extract_images(markdown)
    print(f"  Found {len(images)} images")

    sections = split_markdown_sections(markdown_no_imgs)
    print(f"Split into {len(sections)} sections")

    batches = batch_chunks(sections, args.batch_size)
    print(f"Translating in {len(batches)} batches to {args.to}...")

    translated_sections = []
    for i, batch in enumerate(batches, 1):
        chars = sum(len(s) for s in batch)
        print(f"  [batch {i}/{len(batches)}] {len(batch)} sections, {chars} chars")
        translated_sections.extend(translate_batch(batch, args.to))

    translated_md = "\n\n".join(translated_sections)
    translated_md = restore_images(translated_md, images)

    print(f"Writing {output_path}...")
    with open(output_path, "w", encoding="utf-8") as f:
        f.write(f"*Source: {input_path.name} — translated to {args.to}*\n\n---\n\n")
        f.write(translated_md)

    print(f"Done → {output_path}")

    if not args.no_pdf:
        _generate_pdf(output_path, dark=args.dark)


def _generate_pdf(md_path: Path, dark: bool = False) -> None:
    if not MDTOPDF_SCRIPT.exists():
        print(f"Warning: mdtopdf.sh not found at {MDTOPDF_SCRIPT}, skipping PDF", file=sys.stderr)
        return
    if not shutil.which("node"):
        print("Warning: node not found in PATH, skipping PDF", file=sys.stderr)
        return

    theme = "--dark" if dark else "--light"
    print(f"Generating PDF ({theme[2:]} theme)...")
    try:
        subprocess.run(
            [str(MDTOPDF_SCRIPT), theme, str(md_path.resolve())],
            cwd=str(md_path.parent),
            check=True,
        )
    except subprocess.CalledProcessError as e:
        print(f"Warning: PDF generation failed: {e}", file=sys.stderr)


if __name__ == "__main__":
    main()
