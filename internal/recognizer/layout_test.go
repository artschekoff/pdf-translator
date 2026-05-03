package recognizer

import (
	"testing"

	"github.com/pdf-translator/pdf-translator/internal/domain"
	"github.com/stretchr/testify/assert"
)

func TestMapPaddleLayoutLabel(t *testing.T) {
	tests := []struct {
		label string
		want  string
	}{
		{label: "doc_title", want: domain.BlockTypeTitle},
		{label: "paragraph_title", want: domain.BlockTypeTitle},
		{label: "page_header", want: domain.BlockTypeHeader},
		{label: "page_footer", want: domain.BlockTypeFooter},
		{label: "figure_title", want: domain.BlockTypeCaption},
		{label: "table", want: domain.BlockTypeTable},
		{label: "image", want: domain.BlockTypeImage},
		{label: "text", want: domain.BlockTypeText},
		{label: "unknown_label", want: domain.BlockTypeText},
	}

	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			assert.Equal(t, tt.want, MapPaddleLayoutLabel(tt.label))
		})
	}
}

func TestConvertPaddleLayoutResults(t *testing.T) {
	blocks := convertPaddleLayoutResults([]paddleLayoutBlock{
		{
			Label:      "image",
			Coordinate: []float64{100, 200, 400, 500},
		},
		{
			Label:      "paragraph_title",
			Coordinate: []float64{50, 50, 350, 120},
		},
	}, 842, 300)

	if assert.Len(t, blocks, 2) {
		assert.Equal(t, domain.BlockTypeImage, blocks[0].BlockType)
		assert.InDelta(t, 72.0, blocks[0].BBox.Width, 0.5)
		assert.Equal(t, domain.BlockTypeTitle, blocks[1].BlockType)
		assert.True(t, blocks[1].FontSize > 0)
	}
}
