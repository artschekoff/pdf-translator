package renderer

import (
	"context"
	"testing"

	"github.com/pdf-translator/pdf-translator/internal/domain"
	"github.com/pdf-translator/pdf-translator/internal/extractor"
	"github.com/stretchr/testify/require"
)

// TestNativeExtractionNoImageBlocks verifies that blocks from the native
// extractor (which carry no BlockTypeImage) produce a single text segment,
// not a diagram segment. This is the core invariant of the new approach:
// diagram detection is driven by PaddleOCR's BlockTypeImage, not by gap heuristics.
func TestNativeExtractionNoImageBlocks(t *testing.T) {
	ext := extractor.NewNativeExtractor()
	blocks, err := ext.ExtractPage(context.Background(), "../../examples/bitcoin.pdf", "", 1, 0)
	require.NoError(t, err)
	require.NotEmpty(t, blocks)

	segs := buildSegments(blocks, 792, 612)
	require.NotEmpty(t, segs)
	for _, s := range segs {
		require.False(t, s.isDiagram, "native-only blocks have no BlockTypeImage — must never produce a diagram segment")
	}
}

// TestImageBlockTriggersDiagramSegment verifies that injecting a BlockTypeImage
// block into the block list produces a diagram segment at that position.
func TestImageBlockTriggersDiagramSegment(t *testing.T) {
	ext := extractor.NewNativeExtractor()
	blocks, err := ext.ExtractPage(context.Background(), "../../examples/bitcoin.pdf", "", 2, 0)
	require.NoError(t, err)

	// Inject a fake image block in the middle of the page.
	imageBlock := domain.TextBlock{
		BlockType: domain.BlockTypeImage,
		BBox:      domain.BoundingBox{X: 72, Y: 350, Width: 420, Height: 200},
	}
	blocks = append(blocks, imageBlock)

	segs := buildSegments(blocks, 792, 612)

	diagramCount := 0
	for _, s := range segs {
		if s.isDiagram {
			diagramCount++
		}
	}
	require.Equal(t, 1, diagramCount, "exactly one diagram segment expected for one image block")
}
