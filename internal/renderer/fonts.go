package renderer

import (
	"context"
	"embed"
	"fmt"
	"os"

	"go.uber.org/zap"
)

//go:embed fonts/*.ttf
var embeddedFonts embed.FS

// scriptFontMap maps script names to bundled NotoSans font filenames.
var scriptFontMap = map[string]string{
	"latin":      "NotoSans-Regular.ttf",
	"cyrillic":   "NotoSans-Regular.ttf",
	"arabic":     "NotoSansArabic-Regular.ttf",
	"hebrew":     "NotoSansHebrew-Regular.ttf",
	"devanagari": "NotoSansDevanagari-Regular.ttf",
	"thai":       "NotoSansThai-Regular.ttf",
	"korean":     "NotoSansKR-Regular.ttf",
	"cjk":        "NotoSansSC-Regular.ttf",
}

// FontManager resolves font file paths for a given script.
// User-configured paths (FONT_* in .env) take precedence over bundled fonts.
type FontManager struct {
	overridePaths map[string]string // script → absolute path
}

func NewFontManager(_ string) *FontManager {
	return &FontManager{}
}

func NewFontManagerWithOverrides(_ string, overrides map[string]string) *FontManager {
	return &FontManager{overridePaths: overrides}
}

// FontPath returns the filesystem path for the font matching the given script.
// Resolution order:
//  1. User-configured override path (FONT_* in .env)
//  2. Embedded font (bundled in binary)
func (fm *FontManager) FontPath(_ context.Context, script string) (string, error) {
	if fm.overridePaths != nil {
		if p, ok := fm.overridePaths[script]; ok && p != "" {
			if _, err := os.Stat(p); err == nil {
				return p, nil
			}
		}
	}

	filename, ok := scriptFontMap[script]
	if !ok {
		filename = scriptFontMap["latin"]
	}
	return extractEmbeddedFont(filename)
}

// DownloadAllFonts is a no-op; fonts are bundled in the binary.
func (fm *FontManager) DownloadAllFonts(_ context.Context, logger *zap.Logger) error {
	logger.Info("fonts are bundled in binary — no download needed")
	return nil
}

// extractEmbeddedFont writes the named embedded font to a temp file and
// returns the path. The OS cleans up temp files on reboot; we rely on that
// rather than tracking and deleting them ourselves.
func extractEmbeddedFont(filename string) (string, error) {
	data, err := embeddedFonts.ReadFile("fonts/" + filename)
	if err != nil {
		return "", fmt.Errorf("reading embedded font %s: %w", filename, err)
	}

	tmp, err := os.CreateTemp("", "pdftrans-font-*-"+filename)
	if err != nil {
		return "", fmt.Errorf("creating temp font file: %w", err)
	}
	defer tmp.Close()

	if _, err := tmp.Write(data); err != nil {
		os.Remove(tmp.Name())
		return "", fmt.Errorf("writing font to temp: %w", err)
	}
	return tmp.Name(), nil
}
