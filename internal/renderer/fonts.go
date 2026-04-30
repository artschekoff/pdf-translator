package renderer

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"
)

// scriptFontMap maps script names to NotoSans font variants.
// NotoSans-Regular covers Latin and Cyrillic; specialised variants handle
// other scripts.
var scriptFontMap = map[string]fontInfo{
	"latin":      {filename: "NotoSans-Regular.ttf", url: "https://github.com/google/fonts/raw/main/ofl/notosans/static/NotoSans-Regular.ttf"},
	"cyrillic":   {filename: "NotoSans-Regular.ttf", url: "https://github.com/google/fonts/raw/main/ofl/notosans/static/NotoSans-Regular.ttf"},
	"arabic":     {filename: "NotoSansArabic-Regular.ttf", url: "https://github.com/google/fonts/raw/main/ofl/notosansarabic/NotoSansArabic%5Bwdth%2Cwght%5D.ttf"},
	"hebrew":     {filename: "NotoSansHebrew-Regular.ttf", url: "https://github.com/google/fonts/raw/main/ofl/notosanshebrew/NotoSansHebrew%5Bwdth%2Cwght%5D.ttf"},
	"devanagari": {filename: "NotoSansDevanagari-Regular.ttf", url: "https://github.com/google/fonts/raw/main/ofl/notosansdevanagari/NotoSansDevanagari%5Bwdth%2Cwght%5D.ttf"},
	"thai":       {filename: "NotoSansThai-Regular.ttf", url: "https://github.com/google/fonts/raw/main/ofl/notosansthai/NotoSansThai%5Bwdth%2Cwght%5D.ttf"},
	"korean":     {filename: "NotoSansKR-Regular.ttf", url: "https://github.com/google/fonts/raw/main/ofl/notosanskr/NotoSansKR%5Bwght%5D.ttf"},
	"cjk":        {filename: "NotoSansSC-Regular.ttf", url: "https://github.com/google/fonts/raw/main/ofl/notosanssc/NotoSansSC%5Bwght%5D.ttf"},
}

type fontInfo struct {
	filename string
	url      string
}

// FontManager handles font file resolution and on-demand downloading.
// User-configured paths (from .env FONT_* vars) take precedence over
// downloaded fonts.
type FontManager struct {
	fontsDir      string
	httpClient    *http.Client
	overridePaths map[string]string // script → absolute path from config
}

func NewFontManager(dataDir string) *FontManager {
	return NewFontManagerWithOverrides(dataDir, nil)
}

func NewFontManagerWithOverrides(dataDir string, overrides map[string]string) *FontManager {
	return &FontManager{
		fontsDir: filepath.Join(dataDir, "fonts"),
		httpClient: &http.Client{
			Timeout: 2 * time.Minute,
		},
		overridePaths: overrides,
	}
}

// FontPath returns the filesystem path for the font matching the given script.
// Resolution order:
//  1. User-configured override path (FONT_* in .env)
//  2. Cached font in data directory
//  3. Font bundled next to the binary
//  4. Download from upstream URL
func (fm *FontManager) FontPath(ctx context.Context, script string) (string, error) {
	// 1. User override.
	if fm.overridePaths != nil {
		if p, ok := fm.overridePaths[script]; ok && p != "" {
			if _, err := os.Stat(p); err == nil {
				return p, nil
			}
		}
	}

	info, ok := scriptFontMap[script]
	if !ok {
		info = scriptFontMap["latin"]
	}

	// 2. Cached download.
	cachedPath := filepath.Join(fm.fontsDir, info.filename)
	if _, err := os.Stat(cachedPath); err == nil {
		return cachedPath, nil
	}

	// 3. Bundled next to the binary.
	exePath, exeErr := os.Executable()
	if exeErr != nil {
		exePath = "."
	}
	bundledPath := filepath.Join(filepath.Dir(exePath), "fonts", info.filename)
	if _, err := os.Stat(bundledPath); err == nil {
		return bundledPath, nil
	}

	if info.url == "" {
		return "", fmt.Errorf("font %s not found and no download URL configured", info.filename)
	}

	// 4. Download.
	if err := fm.downloadFont(ctx, info); err != nil {
		return "", fmt.Errorf("downloading font %s: %w", info.filename, err)
	}
	return cachedPath, nil
}

func (fm *FontManager) downloadFont(ctx context.Context, info fontInfo) error {
	if err := os.MkdirAll(fm.fontsDir, 0o755); err != nil {
		return fmt.Errorf("creating fonts dir: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, info.url, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	resp, err := fm.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetching font: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("font download returned status %d", resp.StatusCode)
	}

	path := filepath.Join(fm.fontsDir, info.filename)
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating font file: %w", err)
	}
	defer f.Close()

	const maxFontSize = 100 << 20 // 100 MiB
	if _, err := io.Copy(f, io.LimitReader(resp.Body, maxFontSize)); err != nil {
		os.Remove(path)
		return fmt.Errorf("writing font file: %w", err)
	}
	return nil
}

// DownloadAllFonts pre-downloads all font variants that have a URL configured.
func (fm *FontManager) DownloadAllFonts(ctx context.Context, logger *zap.Logger) error {
	for script, info := range scriptFontMap {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if info.url == "" {
			continue
		}
		path := filepath.Join(fm.fontsDir, info.filename)
		if _, err := os.Stat(path); err == nil {
			logger.Info("font already cached", zap.String("script", script))
			continue
		}
		logger.Info("downloading font", zap.String("script", script), zap.String("file", info.filename))
		if err := fm.downloadFont(ctx, info); err != nil {
			return fmt.Errorf("downloading %s font: %w", script, err)
		}
	}
	return nil
}
