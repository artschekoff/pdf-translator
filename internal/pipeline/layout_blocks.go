package pipeline

import (
	"sort"
	"strings"

	"github.com/pdf-translator/pdf-translator/internal/domain"
)

func mergeLayoutBlocks(layoutBlocks, textBlocks []domain.TextBlock) ([]domain.TextBlock, int) {
	if len(layoutBlocks) == 0 {
		return nil, len(nonEmptyTextBlocks(textBlocks))
	}

	assignments := make([][]domain.TextBlock, len(layoutBlocks))
	textualRegions := make([]int, 0, len(layoutBlocks))
	for i, block := range layoutBlocks {
		if domain.IsTextualBlockType(block.BlockType) {
			textualRegions = append(textualRegions, i)
		}
	}

	unmatched := 0
	var fallback []domain.TextBlock
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
			fallback = append(fallback, tb)
			continue
		}
		assignments[bestIdx] = append(assignments[bestIdx], tb)
	}

	merged := make([]domain.TextBlock, 0, len(layoutBlocks)+len(fallback))
	for i, lb := range layoutBlocks {
		if !domain.IsTextualBlockType(lb.BlockType) {
			merged = append(merged, lb)
			continue
		}

		if len(assignments[i]) == 0 {
			continue
		}

		sortTextBlocks(assignments[i])
		lb.Text = joinTextBlocks(assignments[i])
		if fontSize := averageFontSize(assignments[i]); fontSize > 0 {
			lb.FontSize = fontSize
		}
		lb.FontName = firstFontName(assignments[i])
		merged = append(merged, lb)
	}

	merged = append(merged, fallback...)
	return merged, unmatched
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

func sortTextBlocks(blocks []domain.TextBlock) {
	sort.Slice(blocks, func(i, j int) bool {
		topI := blocks[i].BBox.Y + blocks[i].BBox.Height
		topJ := blocks[j].BBox.Y + blocks[j].BBox.Height
		if topI == topJ {
			return blocks[i].BBox.X < blocks[j].BBox.X
		}
		return topI > topJ
	})
}

func joinTextBlocks(blocks []domain.TextBlock) string {
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		text := strings.TrimSpace(block.Text)
		if text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

func averageFontSize(blocks []domain.TextBlock) float64 {
	total := 0.0
	count := 0
	for _, block := range blocks {
		if block.FontSize <= 0 {
			continue
		}
		total += block.FontSize
		count++
	}
	if count == 0 {
		return 0
	}
	return total / float64(count)
}

func firstFontName(blocks []domain.TextBlock) string {
	for _, block := range blocks {
		if block.FontName != "" {
			return block.FontName
		}
	}
	return ""
}
