package renderer

import (
	"context"
	"os"
	"testing"

	"github.com/pdf-translator/pdf-translator/internal/extractor"
	"github.com/razvandimescu/gopdf/pdf"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestRenderPage_KeepOriginal verifies that enabling keepOriginal doubles
// the output page height compared to the default (translation-only) mode.
func TestRenderPage_KeepOriginal(t *testing.T) {
	const pdfPath = "../../examples/bitcoin.pdf"
	if _, err := os.Stat(pdfPath); err != nil {
		t.Skipf("test PDF not found: %v", err)
	}

	ext := extractor.NewNativeExtractor()
	blocks, err := ext.ExtractPage(context.Background(), pdfPath, "", 1, 0)
	require.NoError(t, err)
	require.NotEmpty(t, blocks)

	for i := range blocks {
		blocks[i].Translated = "translated text"
	}

	rend := NewRenderer(t.TempDir(), zap.NewNop())

	outDefault := t.TempDir() + "/default.pdf"
	outKeep := t.TempDir() + "/keep.pdf"

	require.NoError(t, rend.RenderPage(context.Background(), pdfPath, "", 1, blocks, "en", false, outDefault))
	require.NoError(t, rend.RenderPage(context.Background(), pdfPath, "", 1, blocks, "ru", true, outKeep))

	heightOf := func(path string) float64 {
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		r, err := pdf.Open(data)
		require.NoError(t, err)
		pages, err := r.Pages()
		require.NoError(t, err)
		require.Len(t, pages, 1)
		_, h := pageMediaBox(pages[0])
		return h
	}

	defaultH := heightOf(outDefault)
	keepH := heightOf(outKeep)

	require.Greater(t, keepH, defaultH,
		"keepOriginal output (%.1f) should be taller than default (%.1f)", keepH, defaultH)
}
