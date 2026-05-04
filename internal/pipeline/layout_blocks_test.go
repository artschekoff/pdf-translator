package pipeline

import (
	"testing"

	"github.com/pdf-translator/pdf-translator/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeLayoutBlocks_AssignsTextToLayoutRegions(t *testing.T) {
	layoutBlocks := []domain.TextBlock{
		{
			BBox:      domain.BoundingBox{X: 0, Y: 500, Width: 300, Height: 30},
			BlockType: domain.BlockTypeTitle,
		},
		{
			BBox:      domain.BoundingBox{X: 0, Y: 300, Width: 400, Height: 150},
			BlockType: domain.BlockTypeText,
		},
		{
			BBox:      domain.BoundingBox{X: 320, Y: 250, Width: 180, Height: 180},
			BlockType: domain.BlockTypeImage,
		},
	}

	textBlocks := []domain.TextBlock{
		{
			Text:     "Bitcoin",
			FontSize: 20,
			FontName: "Helvetica-Bold",
			BBox:     domain.BoundingBox{X: 20, Y: 505, Width: 120, Height: 18},
		},
		{
			Text:     "A Peer-to-Peer Electronic Cash System",
			FontSize: 12,
			FontName: "Helvetica",
			BBox:     domain.BoundingBox{X: 10, Y: 380, Width: 360, Height: 14},
		},
		{
			Text:     "Satoshi Nakamoto",
			FontSize: 12,
			FontName: "Helvetica",
			BBox:     domain.BoundingBox{X: 10, Y: 360, Width: 180, Height: 14},
		},
	}

	blocks, unmatched := mergeLayoutBlocks(layoutBlocks, textBlocks)

	// New behaviour: native blocks are kept individually at their original
	// positions; only BlockType is annotated from the layout. Image blocks are
	// appended last so buildSegments can detect diagram regions.
	require.Len(t, blocks, 4) // 3 text blocks (annotated) + 1 image block
	assert.Zero(t, unmatched)

	assert.Equal(t, domain.BlockTypeTitle, blocks[0].BlockType)
	assert.Equal(t, "Bitcoin", blocks[0].Text)

	assert.Equal(t, domain.BlockTypeText, blocks[1].BlockType)
	assert.Equal(t, "A Peer-to-Peer Electronic Cash System", blocks[1].Text)

	assert.Equal(t, domain.BlockTypeText, blocks[2].BlockType)
	assert.Equal(t, "Satoshi Nakamoto", blocks[2].Text)

	assert.Equal(t, domain.BlockTypeImage, blocks[3].BlockType)
	assert.Empty(t, blocks[3].Text)
}

func TestMergeLayoutBlocks_PreservesUnmatchedText(t *testing.T) {
	layoutBlocks := []domain.TextBlock{
		{
			BBox:      domain.BoundingBox{X: 0, Y: 300, Width: 200, Height: 100},
			BlockType: domain.BlockTypeText,
		},
	}

	textBlocks := []domain.TextBlock{
		{
			Text:     "inside",
			FontSize: 12,
			BBox:     domain.BoundingBox{X: 20, Y: 320, Width: 60, Height: 12},
		},
		{
			Text:      "outside",
			FontSize:  12,
			BBox:      domain.BoundingBox{X: 400, Y: 50, Width: 70, Height: 12},
			BlockType: domain.BlockTypeText,
		},
	}

	blocks, unmatched := mergeLayoutBlocks(layoutBlocks, textBlocks)
	require.Len(t, blocks, 2)
	assert.Equal(t, 1, unmatched)
	assert.Equal(t, "inside", blocks[0].Text)
	assert.Equal(t, "outside", blocks[1].Text)
}
