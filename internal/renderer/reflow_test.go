package renderer

import (
	"testing"

	"github.com/pdf-translator/pdf-translator/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildSegments_AttachesTrailingShortLineToParagraph(t *testing.T) {
	pageW := 600.0
	pageH := 800.0
	blocks := []domain.TextBlock{
		{
			Text: "Main paragraph body",
			BBox: domain.BoundingBox{X: 72, Y: 700, Width: 420, Height: 36},
		},
		{
			Text: "stored.",
			BBox: domain.BoundingBox{X: 72, Y: 682, Width: 60, Height: 12},
		},
		{
			Text: "Next paragraph body",
			BBox: domain.BoundingBox{X: 72, Y: 300, Width: 420, Height: 36},
		},
	}

	segs := buildSegments(blocks, pageH, pageW)
	require.Len(t, segs, 3)

	assert.False(t, segs[0].isDiagram)
	assert.Equal(t, 682.0, segs[0].yLow)
	assert.Contains(t, texts(segs[0].blocks), "stored.")

	assert.True(t, segs[1].isDiagram)
	assert.Equal(t, 336.0, segs[1].yLow)
	assert.Equal(t, 682.0, segs[1].yHigh)
	assert.NotContains(t, texts(segs[1].blocks), "stored.")

	assert.False(t, segs[2].isDiagram)
}

func TestBuildSegments_LeavesDistantDiagramLabelInDiagram(t *testing.T) {
	pageW := 600.0
	pageH := 800.0
	blocks := []domain.TextBlock{
		{
			Text: "Main paragraph body",
			BBox: domain.BoundingBox{X: 72, Y: 700, Width: 420, Height: 36},
		},
		{
			Text: "Hash01",
			BBox: domain.BoundingBox{X: 180, Y: 620, Width: 60, Height: 12},
		},
		{
			Text: "Next paragraph body",
			BBox: domain.BoundingBox{X: 72, Y: 300, Width: 420, Height: 36},
		},
	}

	segs := buildSegments(blocks, pageH, pageW)
	require.Len(t, segs, 3)

	assert.True(t, segs[1].isDiagram)
	assert.Contains(t, texts(segs[1].blocks), "Hash01")
	assert.NotContains(t, texts(segs[0].blocks), "Hash01")
	assert.Less(t, segs[1].renderLow, segs[1].yLow)
	assert.Greater(t, segs[1].renderHigh, segs[1].yHigh)
	assert.Greater(t, segs[1].renderHigh, segs[0].yLow)
	assert.Less(t, segs[1].renderLow, segs[2].yHigh)
}

func texts(blocks []domain.TextBlock) []string {
	result := make([]string, 0, len(blocks))
	for _, b := range blocks {
		result = append(result, b.Text)
	}
	return result
}
