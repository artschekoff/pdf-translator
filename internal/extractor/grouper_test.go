package extractor

import (
	"testing"

	"github.com/pdf-translator/pdf-translator/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGroupLines_Empty(t *testing.T) {
	blocks := groupLines(nil, 1)
	assert.Nil(t, blocks)
}

func TestGroupLines_SingleLine(t *testing.T) {
	lines := []textLine{
		{Text: "Hello world", X: 72, Y: 700, Width: 100, Height: 12, FontSize: 12, FontName: "Helvetica"},
	}

	blocks := groupLines(lines, 1)
	require.Len(t, blocks, 1)
	assert.Equal(t, "Hello world", blocks[0].Text)
	assert.Equal(t, 1, blocks[0].PageNum)
}

func TestGroupLines_TwoAdjacentLines(t *testing.T) {
	lines := []textLine{
		{Text: "Line one", X: 72, Y: 700, Width: 100, Height: 12, FontSize: 12, FontName: "Helvetica"},
		{Text: "Line two", X: 72, Y: 714, Width: 100, Height: 12, FontSize: 12, FontName: "Helvetica"},
	}

	blocks := groupLines(lines, 1)
	require.Len(t, blocks, 1)
	assert.Contains(t, blocks[0].Text, "Line one")
	assert.Contains(t, blocks[0].Text, "Line two")
}

func TestGroupLines_SplitByLargeGap(t *testing.T) {
	lines := []textLine{
		{Text: "Paragraph one", X: 72, Y: 700, Width: 100, Height: 12, FontSize: 12, FontName: "Helvetica"},
		{Text: "Paragraph two", X: 72, Y: 800, Width: 100, Height: 12, FontSize: 12, FontName: "Helvetica"},
	}

	blocks := groupLines(lines, 1)
	require.Len(t, blocks, 2)
	assert.Equal(t, "Paragraph one", blocks[0].Text)
	assert.Equal(t, "Paragraph two", blocks[1].Text)
}

func TestGroupLines_SplitByFontSizeChange(t *testing.T) {
	lines := []textLine{
		{Text: "Title", X: 72, Y: 700, Width: 100, Height: 24, FontSize: 24, FontName: "Helvetica"},
		{Text: "Body text", X: 72, Y: 730, Width: 100, Height: 12, FontSize: 12, FontName: "Helvetica"},
	}

	blocks := groupLines(lines, 1)
	require.Len(t, blocks, 2)
	assert.Equal(t, "Title", blocks[0].Text)
	assert.Equal(t, domain.BlockTypeTitle, blocks[0].BlockType)
	assert.Equal(t, "Body text", blocks[1].Text)
	assert.Equal(t, domain.BlockTypeText, blocks[1].BlockType)
}

func TestOverlaps(t *testing.T) {
	assert.True(t, overlaps(0, 100, 50, 150))
	assert.True(t, overlaps(0, 100, 0, 100))
	assert.False(t, overlaps(0, 100, 200, 300))
	assert.False(t, overlaps(0, 100, 100, 200)) // touching but not overlapping
}
