package renderer

import (
	"context"
	"testing"

	"github.com/pdf-translator/pdf-translator/internal/extractor"
	"github.com/stretchr/testify/require"
)

func TestDebugPage2Segments(t *testing.T) {
	ext := extractor.NewNativeExtractor()
	blocks, err := ext.ExtractPage(context.Background(), "../../examples/bitcoin.pdf", "", 2, 0)
	require.NoError(t, err)

	segs := buildSegments(blocks, 792, 612)
	require.GreaterOrEqual(t, len(segs), 3)
	require.False(t, segs[0].isDiagram)
	require.True(t, segs[1].isDiagram)
	require.False(t, segs[2].isDiagram)
}

func TestPage1TitleBlockDoesNotCreateDiagramSegment(t *testing.T) {
	ext := extractor.NewNativeExtractor()
	blocks, err := ext.ExtractPage(context.Background(), "../../examples/bitcoin.pdf", "", 1, 0)
	require.NoError(t, err)

	segs := buildSegments(blocks, 792, 612)
	require.Len(t, segs, 2)
	require.False(t, segs[0].isDiagram)
	require.False(t, segs[1].isDiagram)
}
