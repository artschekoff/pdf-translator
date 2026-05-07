package pipeline

import (
	"math"
	"sort"
	"strings"

	"github.com/pdf-translator/pdf-translator/internal/domain"
)

// mergeLayoutBlocks groups individual text lines into one block per layout region.
// Lines whose center falls within a textual region are collected, sorted into
// reading order, and merged into a single TextBlock. Unmatched lines are kept
// as-is. Image-type regions are appended so that buildSegments can detect
// diagram regions.
func mergeLayoutBlocks(layoutBlocks, textBlocks []domain.TextBlock) ([]domain.TextBlock, int) {
	if len(layoutBlocks) == 0 {
		return nonEmptyTextBlocks(textBlocks), 0
	}

	assigned := make([]bool, len(textBlocks))
	var result []domain.TextBlock

	for _, region := range layoutBlocks {
		if !domain.IsTextualBlockType(region.BlockType) {
			continue
		}
		// Skip super-regions: layout blocks whose BBox contains the center of
		// another layout block. These are grouping containers (e.g. "reference"
		// enclosing individual "reference_content" entries) — processing them
		// would create an extra block that overlaps with the sub-region blocks,
		// causing doubled text in the renderer.
		if containsOtherRegionCenter(region.BBox, layoutBlocks) {
			continue
		}

		var regionLines []domain.TextBlock
		for i, tb := range textBlocks {
			if strings.TrimSpace(tb.Text) == "" {
				continue
			}
			cx := tb.BBox.X + tb.BBox.Width/2
			cy := tb.BBox.Y + tb.BBox.Height/2
			if cx >= region.BBox.X && cx <= region.BBox.X+region.BBox.Width &&
				cy >= region.BBox.Y && cy <= region.BBox.Y+region.BBox.Height {
				regionLines = append(regionLines, tb)
				assigned[i] = true
			}
		}

		if len(regionLines) == 0 {
			continue
		}

		result = append(result, mergeTextLines(regionLines, region.BlockType))
	}

	// Collect non-textual regions (formula, image, etc.) to exclude text lines
	// that fall within them — those lines belong to the formula/figure and must
	// not be translated.
	var nonTextualRegions []domain.BoundingBox
	for _, lb := range layoutBlocks {
		if !domain.IsTextualBlockType(lb.BlockType) {
			nonTextualRegions = append(nonTextualRegions, lb.BBox)
		}
	}

	unmatched := 0
	for i, tb := range textBlocks {
		if assigned[i] || strings.TrimSpace(tb.Text) == "" {
			continue
		}
		cx := tb.BBox.X + tb.BBox.Width/2
		cy := tb.BBox.Y + tb.BBox.Height/2
		inFormula := false
		for _, r := range nonTextualRegions {
			if cx >= r.X && cx <= r.X+r.Width && cy >= r.Y && cy <= r.Y+r.Height {
				inFormula = true
				break
			}
		}
		if inFormula {
			continue
		}
		unmatched++
		result = append(result, tb)
	}

	for _, lb := range layoutBlocks {
		if !domain.IsTextualBlockType(lb.BlockType) {
			result = append(result, lb)
		}
	}

	return result, unmatched
}

// mergeTextLines sorts lines into reading order and joins them into one TextBlock.
func mergeTextLines(lines []domain.TextBlock, blockType string) domain.TextBlock {
	sort.Slice(lines, func(i, j int) bool {
		yi, yj := lines[i].BBox.Y, lines[j].BBox.Y
		if math.Abs(yi-yj) > 2 {
			return yi > yj
		}
		return lines[i].BBox.X < lines[j].BBox.X
	})

	minX := lines[0].BBox.X
	minY := lines[0].BBox.Y
	maxX := lines[0].BBox.X + lines[0].BBox.Width
	maxY := lines[0].BBox.Y + lines[0].BBox.Height

	var texts []string
	var totalFontSize float64
	for _, l := range lines {
		texts = append(texts, l.Text)
		totalFontSize += l.FontSize
		if l.BBox.X < minX {
			minX = l.BBox.X
		}
		if l.BBox.Y < minY {
			minY = l.BBox.Y
		}
		if l.BBox.X+l.BBox.Width > maxX {
			maxX = l.BBox.X + l.BBox.Width
		}
		if l.BBox.Y+l.BBox.Height > maxY {
			maxY = l.BBox.Y + l.BBox.Height
		}
	}

	return domain.TextBlock{
		PageNum: lines[0].PageNum,
		BBox: domain.BoundingBox{
			X:      minX,
			Y:      minY,
			Width:  maxX - minX,
			Height: maxY - minY,
		},
		Text:      strings.Join(texts, " "),
		FontSize:  totalFontSize / float64(len(lines)),
		FontName:  lines[0].FontName,
		BlockType: blockType,
	}
}

// containsOtherRegionCenter returns true when outer's BBox contains the center
// of any OTHER block in the slice. Such a region is a "super-region" container
// (e.g. a "reference" block enclosing eight "reference_content" blocks) and
// should be skipped so that only the inner leaf regions produce merged blocks.
func containsOtherRegionCenter(outer domain.BoundingBox, all []domain.TextBlock) bool {
	for _, other := range all {
		cx := other.BBox.X + other.BBox.Width/2
		cy := other.BBox.Y + other.BBox.Height/2
		if cx == outer.X+outer.Width/2 && cy == outer.Y+outer.Height/2 {
			continue // same region (center coincides exactly)
		}
		if cx > outer.X && cx < outer.X+outer.Width &&
			cy > outer.Y && cy < outer.Y+outer.Height {
			return true
		}
	}
	return false
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
