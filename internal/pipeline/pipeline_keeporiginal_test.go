package pipeline_test

import (
	"context"
	"testing"

	"github.com/pdf-translator/pdf-translator/internal/domain"
	"github.com/pdf-translator/pdf-translator/internal/pipeline"
	"github.com/pdf-translator/pdf-translator/internal/queue"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type mockExtractor struct{ blocks []domain.TextBlock }

func (m *mockExtractor) ExtractPage(_ context.Context, _, _ string, _ int, _ int) ([]domain.TextBlock, error) {
	return m.blocks, nil
}

type mockTranslator struct{}

func (m *mockTranslator) TranslateBlocks(_ context.Context, blocks []domain.TextBlock, _, _ string) ([]domain.TextBlock, error) {
	for i := range blocks {
		blocks[i].Translated = "translated"
	}
	return blocks, nil
}

type captureRenderer struct {
	capturedKeepOriginal bool
}

func (r *captureRenderer) RenderPage(_ context.Context, _, _ string, _ int, _ []domain.TextBlock, _ string, keepOriginal bool, _ string) error {
	r.capturedKeepOriginal = keepOriginal
	return nil
}

func TestProcessPage_PassesKeepOriginalToRenderer(t *testing.T) {
	rend := &captureRenderer{}
	blocks := []domain.TextBlock{
		{Text: "hello", BBox: domain.BoundingBox{X: 0, Y: 0, Width: 100, Height: 20}, BlockType: domain.BlockTypeText},
	}
	ext := &mockExtractor{blocks: blocks}
	pipe := pipeline.New(ext, ext, ext, &mockTranslator{}, rend, zap.NewNop())

	pageJob := &queue.PageJob{PageNum: 1, PageType: "native"}
	docJob := &queue.DocumentJob{
		InputPath:    "../../examples/bitcoin.pdf",
		TargetLang:   "ru",
		SourceLang:   "en",
		DPI:          300,
		TempDir:      t.TempDir(),
		KeepOriginal: true,
	}

	err := pipe.ProcessPage(context.Background(), pageJob, docJob)
	require.NoError(t, err)
	require.True(t, rend.capturedKeepOriginal)
}
