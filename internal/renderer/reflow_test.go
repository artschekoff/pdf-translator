package renderer

import (
	"testing"

	"github.com/pdf-translator/pdf-translator/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildSegments_PureTextPage_SingleTextSegment(t *testing.T) {
	blocks := []domain.TextBlock{
		{Text: "Introduction", BlockType: domain.BlockTypeTitle, BBox: domain.BoundingBox{X: 72, Y: 700, Width: 420, Height: 20}},
		{Text: "Some body text here.", BlockType: domain.BlockTypeText, BBox: domain.BoundingBox{X: 72, Y: 500, Width: 420, Height: 100}},
		{Text: "Another paragraph.", BlockType: domain.BlockTypeText, BBox: domain.BoundingBox{X: 72, Y: 300, Width: 420, Height: 80}},
	}
	segs := buildSegments(blocks, 800, 600)
	require.Len(t, segs, 1)
	assert.False(t, segs[0].isDiagram)
	assert.Len(t, segs[0].blocks, 3)
}

func TestBuildSegments_NarrowBlocks_StillTextSegment(t *testing.T) {
	// Narrow bullet-point blocks that would have triggered the old width heuristic.
	blocks := []domain.TextBlock{
		{Text: "• Item one", BlockType: domain.BlockTypeText, BBox: domain.BoundingBox{X: 72, Y: 700, Width: 80, Height: 12}},
		{Text: "• Item two", BlockType: domain.BlockTypeText, BBox: domain.BoundingBox{X: 72, Y: 680, Width: 80, Height: 12}},
		{Text: "• Item three", BlockType: domain.BlockTypeText, BBox: domain.BoundingBox{X: 72, Y: 660, Width: 80, Height: 12}},
	}
	segs := buildSegments(blocks, 800, 600)
	require.Len(t, segs, 1)
	assert.False(t, segs[0].isDiagram, "narrow text blocks must NOT trigger diagram mode")
}

func TestBuildSegments_ImageBlock_CreatesDiagramSegment(t *testing.T) {
	blocks := []domain.TextBlock{
		{Text: "Introduction", BlockType: domain.BlockTypeTitle, BBox: domain.BoundingBox{X: 72, Y: 700, Width: 420, Height: 20}},
		{BlockType: domain.BlockTypeImage, BBox: domain.BoundingBox{X: 72, Y: 400, Width: 420, Height: 200}},
		{Text: "Figure 1: Diagram", BlockType: domain.BlockTypeCaption, BBox: domain.BoundingBox{X: 72, Y: 380, Width: 200, Height: 12}},
		{Text: "Body text after figure.", BlockType: domain.BlockTypeText, BBox: domain.BoundingBox{X: 72, Y: 200, Width: 420, Height: 80}},
	}
	segs := buildSegments(blocks, 800, 600)
	require.GreaterOrEqual(t, len(segs), 2)

	// Find the diagram segment.
	diagramIdx := -1
	for i, s := range segs {
		if s.isDiagram {
			diagramIdx = i
			break
		}
	}
	require.NotEqual(t, -1, diagramIdx, "must have at least one diagram segment")

	// Surrounding segments must be text.
	if diagramIdx > 0 {
		assert.False(t, segs[diagramIdx-1].isDiagram)
	}
	if diagramIdx < len(segs)-1 {
		assert.False(t, segs[diagramIdx+1].isDiagram)
	}
}

func TestBuildSegments_MultipleImages_AlternateSegments(t *testing.T) {
	blocks := []domain.TextBlock{
		{Text: "Top text", BlockType: domain.BlockTypeText, BBox: domain.BoundingBox{X: 72, Y: 750, Width: 420, Height: 30}},
		{BlockType: domain.BlockTypeImage, BBox: domain.BoundingBox{X: 72, Y: 550, Width: 420, Height: 150}},
		{Text: "Middle text", BlockType: domain.BlockTypeText, BBox: domain.BoundingBox{X: 72, Y: 400, Width: 420, Height: 80}},
		{BlockType: domain.BlockTypeImage, BBox: domain.BoundingBox{X: 72, Y: 150, Width: 420, Height: 200}},
		{Text: "Bottom text", BlockType: domain.BlockTypeText, BBox: domain.BoundingBox{X: 72, Y: 50, Width: 420, Height: 60}},
	}
	segs := buildSegments(blocks, 800, 600)

	diagramCount := 0
	for _, s := range segs {
		if s.isDiagram {
			diagramCount++
		}
	}
	assert.Equal(t, 2, diagramCount, "should create one diagram segment per image region")

	// No two diagram segments should be adjacent.
	for i := 1; i < len(segs); i++ {
		assert.False(t, segs[i-1].isDiagram && segs[i].isDiagram, "adjacent diagram segments")
	}
}

func TestBuildSegments_EmptyBlocks_ReturnsNil(t *testing.T) {
	assert.Nil(t, buildSegments(nil, 800, 600))
	assert.Nil(t, buildSegments([]domain.TextBlock{}, 800, 600))
}

func texts(blocks []domain.TextBlock) []string {
	result := make([]string, 0, len(blocks))
	for _, b := range blocks {
		result = append(result, b.Text)
	}
	return result
}
