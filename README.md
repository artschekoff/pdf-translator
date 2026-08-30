<div align="center">

# 📄 PDF Translator

**Translate any PDF while preserving its original layout and formatting**

[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?style=for-the-badge&logo=go&logoColor=white)](https://golang.org)
[![OpenAI](https://img.shields.io/badge/OpenAI-GPT--4o--mini-412991?style=for-the-badge&logo=openai&logoColor=white)](https://openai.com)
[![Docker](https://img.shields.io/badge/Docker-Ready-2496ED?style=for-the-badge&logo=docker&logoColor=white)](https://docker.com)
[![License](https://img.shields.io/badge/License-MIT-green?style=for-the-badge)](LICENSE)

</div>

---

## The Problem

Translating PDF documents is painful. Existing tools either:

- 🚫 **Destroy the layout** — pasting translated text as plain paragraphs, losing all formatting
- 🚫 **Can't handle scanned PDFs** — image-based documents are completely skipped
- 🚫 **Break on complex layouts** — multi-column pages, tables, and mixed-language content come out garbled
- 🚫 **Require manual copy-paste** — no way to automate batch translation
- 🚫 **Have no resumability** — a failure halfway through means starting over

PDF Translator solves all of this.

---

## What It Does

```
┌─────────────────────┐     ┌──────────────────────────────────────────────────┐
│   Original PDF      │     │              Translated PDF                      │
│                     │     │                                                  │
│  Hello, World!      │────▶│  ¡Hola, Mundo!                                  │
│  ┌───────────────┐  │     │  ┌───────────────┐                               │
│  │  Figure 1     │  │     │  │  Figure 1     │                               │
│  │  [image]      │  │     │  │  [image]      │                               │
│  └───────────────┘  │     │  └───────────────┘                               │
│  Caption text here  │     │  Texto del pie aquí                              │
│                     │     │                                                  │
└─────────────────────┘     └──────────────────────────────────────────────────┘
         ↑ layout preserved, text translated, images untouched
```

It **translates the text in-place** — covering the original with white rectangles and overlaying the translated text at the exact same position, preserving every visual element.

---

## Features

### 🔍 Smart Page Detection
Automatically distinguishes between native PDFs (selectable text) and scanned PDFs (image-only pages). Each page type is handled optimally — no configuration required.

### 🤖 Dual OCR Engine Support
Scanned pages are processed through your choice of OCR backend:
| Engine | Best For |
|--------|----------|
| **PaddleOCR** (default) | High accuracy, multi-language, runs in Docker |
| **Tesseract** | Lightweight alternative, widely supported |

### 🌍 Powered by GPT-4o-mini
Translations use OpenAI's GPT-4o-mini for high-quality, context-aware results. Blocks are sent in batches to minimize API cost, with automatic per-block fallback and retry on failure.

### 📐 Layout-Preserving Rendering
- Translated text is placed at the **exact pixel position** of the original
- Original text is covered with **background-matched white rectangles**
- **Bidirectional text** (Arabic, Hebrew, RTL languages) rendered correctly
- **Multi-language fonts** downloaded on demand from Google Fonts

### ⚡ Parallel Processing
Pages are processed concurrently with a configurable worker pool. A 100-page document doesn't take 100× longer than a single page.

### 💾 Resumable Jobs
Every job is tracked in a local **SQLite database**. If a translation fails mid-way (network error, API timeout), just run the same command again — it picks up where it left off.

### 🐳 Zero-Setup Docker Mode
Run the entire stack — translator + OCR services — with a single `make docker-run` command. No local dependencies needed.

---

## Quick Start

### Option A: Docker (Recommended)

No local Go or MuPDF installation required.

```bash
# 1. Clone and configure
git clone https://github.com/artschekoff/pdf-translator
cd pdf-translator
cp .env.example .env
# Edit .env and set OPENAI_API_KEY=sk-...

# 2. Translate
make docker-run INPUT=my-document.pdf TO=spanish
```

The translated PDF is saved to `./output/`.

---

### Option B: Local Binary

#### macOS

```bash
# No extra prerequisites — MuPDF is bundled

# First-time setup
make setup-mac

# Build & translate
make run-mac ARGS="translate my-document.pdf --to spanish"
```

#### Windows

```bash
# Prerequisites: MSYS2 + MinGW-w64
winget install -e --id MSYS2.MSYS2
# In MSYS2 shell:
pacman -S mingw-w64-x86_64-gcc mingw-w64-x86_64-libmupdf

# First-time setup
make setup

# Build & translate
make run-win ARGS="translate my-document.pdf --to spanish"
```

---

## Usage

```bash
# Translate to Spanish
pdf-translator translate document.pdf --to spanish

# Translate to Japanese with explicit source language
pdf-translator translate document.pdf --from english --to japanese

# Process specific pages only
pdf-translator translate document.pdf --to french --pages 1-10

# Use more parallel workers (default: 4)
pdf-translator translate document.pdf --to german --workers 8

# Custom output path
pdf-translator translate document.pdf --to italian --output ./translated/doc_it.pdf
```

---

## Configuration

Copy `.env.example` to `.env` and set your values:

```env
# Required
OPENAI_API_KEY=sk-...

# OCR Services (started via make services-up)
PADDLE_OCR_URL=http://localhost:8051
TESSERACT_URL=http://localhost:8052
OCR_ENGINE=paddleocr          # paddleocr | tesseract

# Performance
MAX_PAGE_WORKERS=4            # parallel page processing
DPI=300                       # render DPI for scanned pages
```

---

## Architecture

```
PDF Input
    │
    ▼
┌─────────────┐
│  Detector   │  Classifies each page: native text vs. scanned image
└──────┬──────┘
       │
    ┌──┴──┐
    │     │
    ▼     ▼
┌───────┐ ┌───────────┐
│Native │ │  OCR      │  PaddleOCR or Tesseract (HTTP services)
│Extract│ │  Extract  │
└───┬───┘ └─────┬─────┘
    └─────┬─────┘
          │  TextBlocks (position + content)
          ▼
    ┌───────────┐
    │Translator │  Batched GPT-4o-mini calls with retry logic
    └─────┬─────┘
          │  TranslatedBlocks
          ▼
    ┌───────────┐
    │ Renderer  │  White-rect masking + translated text overlay
    └─────┬─────┘
          │
          ▼
    ┌───────────┐
    │ Assembler │  Joins translated pages into final PDF
    └─────┬─────┘
          │
          ▼
    PDF Output
```

All jobs are persisted in SQLite for resumability and retry handling.

---

## Make Targets

| Target | Description |
|--------|-------------|
| `build-mac` | Build binary for macOS (requires Homebrew MuPDF) |
| `build-win` | Build binary for Windows (requires MSYS2 MuPDF) |
| `run-mac ARGS="..."` | Build and run on macOS |
| `run-win ARGS="..."` | Build and run on Windows |
| `build-all` | Cross-compile for all platforms |
| `services-up` | Start PaddleOCR in Docker |
| `services-up-all` | Start PaddleOCR + Tesseract in Docker |
| `docker-run INPUT=f.pdf TO=lang` | Full Docker translation |
| `health` | Check if OCR services are reachable |
| `validate` | Run fmt → vet → lint → test |

Run `make help` for the full list.

---

## Supported Languages

Any language supported by GPT-4o-mini and your chosen OCR engine. Common targets:

`spanish` · `french` · `german` · `italian` · `portuguese` · `japanese` · `chinese` · `korean` · `arabic` · `russian` · `hindi` · `dutch` · `polish` · `turkish` · `vietnamese`

For RTL languages (Arabic, Hebrew), bidirectional text rendering is handled automatically.

---

## Requirements

| Component | Requirement |
|-----------|------------|
| OpenAI API key | GPT-4o-mini access |
| Docker | For OCR services (PaddleOCR / Tesseract) |
| Go 1.22+ | For local builds only |
| MuPDF | For local builds only (`brew install mupdf` / MSYS2) |

---

<div align="center">

Built with Go · OpenAI · PaddleOCR · MuPDF

</div>
