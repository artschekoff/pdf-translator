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

// Font variant mappings for Noto Sans family.
var scriptFontMap = map[string]fontInfo{
	"latin":      {filename: "NotoSans-Regular.ttf", url: "https://github.com/google/fonts/raw/main/ofl/notosans/NotoSans%5Bwdth%2Cwght%5D.ttf"},
	"cyrillic":   {filename: "NotoSans-Regular.ttf", url: ""},
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
type FontManager struct {
	fontsDir   string
	httpClient *http.Client
}

func NewFontManager(dataDir string) *FontManager {
	return &FontManager{
		fontsDir: filepath.Join(dataDir, "fonts"),
		httpClient: &http.Client{
			Timeout: 2 * time.Minute,
		},
	}
}

// FontPath returns the filesystem path for the font matching the given script.
// Downloads the font on first use if not cached.
func (fm *FontManager) FontPath(ctx context.Context, script string) (string, error) {
	info, ok := scriptFontMap[script]
	if !ok {
		info = scriptFontMap["latin"]
	}

	path := filepath.Join(fm.fontsDir, info.filename)
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}

	// Check bundled fonts directory next to the binary.
	exePath, exeErr := os.Executable()
	if exeErr != nil {
		exePath = "."
	}
	bundledPath := filepath.Join(filepath.Dir(exePath), "fonts", info.filename)
	if _, err := os.Stat(bundledPath); err == nil {
		return bundledPath, nil
	}

	if info.url == "" {
		return "", fmt.Errorf("font %s not found and no download URL available", info.filename)
	}

	if err := fm.downloadFont(ctx, info); err != nil {
		return "", fmt.Errorf("downloading font %s: %w", info.filename, err)
	}

	return path, nil
}

func (fm *FontManager) downloadFont(ctx context.Context, info fontInfo) error {
	if err := os.MkdirAll(fm.fontsDir, 0o755); err != nil {
		return fmt.Errorf("creating fonts dir: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, info.url, nil)
	if err != nil {
		return fmt.Errorf("creating font request: %w", err)
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

// DownloadAllFonts pre-downloads all font variants.
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
