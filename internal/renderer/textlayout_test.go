package renderer

import (
	"testing"

	"github.com/pdf-translator/pdf-translator/internal/domain"
	"github.com/stretchr/testify/assert"
)

func TestFitText_FitsAtOriginalSize(t *testing.T) {
	bbox := domain.BoundingBox{X: 0, Y: 0, Width: 200, Height: 50}
	result := FitText("Short text", bbox, 12)

	assert.False(t, result.Overflow)
	assert.Equal(t, 12.0, result.FontSize)
	assert.NotEmpty(t, result.Lines)
}

func TestFitText_WrapsLongText(t *testing.T) {
	bbox := domain.BoundingBox{X: 0, Y: 0, Width: 100, Height: 100}
	longText := "This is a very long text that should definitely need word wrapping to fit within the bounding box"
	result := FitText(longText, bbox, 12)

	assert.True(t, len(result.Lines) > 1, "expected multiple lines")
}

func TestFitText_ShrinksFont(t *testing.T) {
	bbox := domain.BoundingBox{X: 0, Y: 0, Width: 50, Height: 20}
	text := "This text is way too long for this tiny box and needs shrinking"
	result := FitText(text, bbox, 14)

	assert.True(t, result.FontSize < 14, "expected font size to shrink")
}

func TestFitText_TruncatesAsLastResort(t *testing.T) {
	bbox := domain.BoundingBox{X: 0, Y: 0, Width: 30, Height: 10}
	text := "Extremely long text that absolutely cannot fit in this minuscule bounding box no matter what we do with font sizes and wrapping"
	result := FitText(text, bbox, 12)

	assert.True(t, result.Overflow)
	assert.NotEmpty(t, result.Lines)
}

func TestFitText_ExpandsBoxHeightForModerateOverflow(t *testing.T) {
	bbox := domain.BoundingBox{X: 0, Y: 0, Width: 90, Height: 20}
	text := "Bitcoin A Peer to Peer Electronic Cash System Whitepaper"

	result := FitText(text, bbox, 14)

	assert.True(t, result.Overflow)
	assert.Greater(t, result.BoxHeight, bbox.Height)
	assert.GreaterOrEqual(t, len(result.Lines), 2)
}

func TestFitText_PreservesNewlines(t *testing.T) {
	bbox := domain.BoundingBox{X: 0, Y: 0, Width: 200, Height: 100}
	text := "Line one\nLine two\nLine three"
	result := FitText(text, bbox, 12)

	assert.GreaterOrEqual(t, len(result.Lines), 3)
}

func TestWrapText_SingleWord(t *testing.T) {
	lines := wrapText("Hello", 200, 12, avgCharWidthRatioLat)
	assert.Equal(t, []string{"Hello"}, lines)
}

func TestWrapText_EmptyString(t *testing.T) {
	lines := wrapText("", 200, 12, avgCharWidthRatioLat)
	assert.Equal(t, []string{""}, lines)
}

func TestCharWidthRatioForText(t *testing.T) {
	assert.Equal(t, avgCharWidthRatioLat, charWidthRatioForText("Hello world"))
	assert.Equal(t, avgCharWidthRatioCJK, charWidthRatioForText("你好世界"))
	assert.Equal(t, avgCharWidthRatioCJK, charWidthRatioForText("안녕하세요"))
}

func TestMeasureTextWidth(t *testing.T) {
	width := MeasureTextWidth("Hello", 12)
	assert.Greater(t, width, 0.0)
}
