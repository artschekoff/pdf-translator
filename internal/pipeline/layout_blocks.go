package pipeline

import (
	"strings"

	"github.com/pdf-translator/pdf-translator/internal/domain"
)

// mergeLayoutBlocks enriches native text blocks with PaddleOCR layout metadata.
//
// Old approach: merge all native blocks within a layout region into one block,
// losing paragraph spacing. New approach: keep each native block at its original
// position and BBox, annotating it with the matching layout block's BlockType.
// Image-type layout blocks (figures, charts) are appended as-is so that
// buildSegments can detect diagram regions.
func mergeLayoutBlocks(layoutBlocks, textBlocks []domain.TextBlock) ([]domain.TextBlock, int) {
	if len(layoutBlocks) == 0 {
		return nonEmptyTextBlocks(textBlocks), 0
	}

	textualRegions := make([]int, 0, len(layoutBlocks))
	for i, block := range layoutBlocks {
		if domain.IsTextualBlockType(block.BlockType) {
			textualRegions = append(textualRegions, i)
		}
	}

	var result []domain.TextBlock
	unmatched := 0

	for _, tb := range textBlocks {
		if strings.TrimSpace(tb.Text) == "" {
			continue
		}

		bestIdx := -1
		bestScore := 0.0
		for _, idx := range textualRegions {
			score := layoutAssignmentScore(tb.BBox, layoutBlocks[idx].BBox)
			if score > bestScore {
				bestScore = score
				bestIdx = idx
			}
		}

		if bestIdx == -1 {
			unmatched++
			result = append(result, tb)
			continue
		}

		// Annotate with layout metadata but preserve the native block's position.
		annotated := tb
		annotated.BlockType = layoutBlocks[bestIdx].BlockType
		result = append(result, annotated)
	}

	// Append image-type layout blocks (no text, but needed for diagram detection).
	for _, lb := range layoutBlocks {
		if !domain.IsTextualBlockType(lb.BlockType) {
			result = append(result, lb)
		}
	}

	return result, unmatched
}

func nonEmptyTextBlocks(blocks []domain.TextBlock) []domain.TextBlock {
	filtered := make([]domain.TextBlock, 0, len(blocks))
	for _, block := range blocks {
		if strings.TrimSpace(block.Text) != "" {
			filtered = append(filtered, block)
		}
	}
	return filtered
}

func layoutAssignmentScore(textBox, regionBox domain.BoundingBox) float64 {
	overlap := bboxOverlap(textBox, regionBox)
	if overlap > 0 {
		score := overlap
		if bboxContainsCenter(regionBox, textBox) {
			score += 1
		}
		return score
	}
	if bboxContainsCenter(regionBox, textBox) {
		return 0.5
	}
	return 0
}

func bboxContainsCenter(outer, inner domain.BoundingBox) bool {
	cx := inner.X + inner.Width/2
	cy := inner.Y + inner.Height/2
	return cx >= outer.X && cx <= outer.X+outer.Width && cy >= outer.Y && cy <= outer.Y+outer.Height
}
