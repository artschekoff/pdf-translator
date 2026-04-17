package recognizer

import (
	"testing"

	"github.com/pdf-translator/pdf-translator/internal/domain"
	"github.com/stretchr/testify/assert"
)

func TestPixelToPDFPoint(t *testing.T) {
	tests := []struct {
		name     string
		px       float64
		dpi      int
		expected float64
	}{
		{"300 DPI - 300px", 300, 300, 72.0},
		{"300 DPI - 150px", 150, 300, 36.0},
		{"150 DPI - 150px", 150, 150, 72.0},
		{"72 DPI - 72px", 72, 72, 72.0},
		{"300 DPI - 0px", 0, 300, 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := PixelToPDFPoint(tt.px, tt.dpi)
			assert.InDelta(t, tt.expected, result, 0.01)
		})
	}
}

func TestClassifyBlock(t *testing.T) {
	tests := []struct {
		name          string
		pdfHeight     float64
		wantFontSize  float64
		wantBlockType string
	}{
		{"small text", 12.0, 9.0, domain.BlockTypeText},
		{"normal text", 16.0, 12.0, domain.BlockTypeText},
		{"title threshold", 22.0, 16.5, domain.BlockTypeTitle},
		{"large title", 40.0, 30.0, domain.BlockTypeTitle},
		{"zero height", 0.0, 0.0, domain.BlockTypeText},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fontSize, blockType := ClassifyBlock(tt.pdfHeight)
			assert.InDelta(t, tt.wantFontSize, fontSize, 0.01)
			assert.Equal(t, tt.wantBlockType, blockType)
		})
	}
}
