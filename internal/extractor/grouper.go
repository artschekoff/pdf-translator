package extractor

import (
	"math"
	"sort"
	"strings"

	"github.com/pdf-translator/pdf-translator/internal/domain"
)

type textLine struct {
	Text     string
	X        float64
	Y        float64
	Width    float64
	Height   float64
	FontSize float64
	FontName string
}

// groupLines clusters individual text lines into logical text blocks
// by spatial proximity, font characteristics, and indentation.
func groupLines(lines []textLine, pageNum int) []domain.TextBlock {
	if len(lines) == 0 {
		return nil
	}

	// Descending Y = top-to-bottom reading order (PDF Y goes up from page bottom).
	sort.Slice(lines, func(i, j int) bool {
		if math.Abs(lines[i].Y-lines[j].Y) < 2 {
			return lines[i].X < lines[j].X
		}
		return lines[i].Y > lines[j].Y
	})

	var blocks []domain.TextBlock
	var current []textLine
	current = append(current, lines[0])

	for i := 1; i < len(lines); i++ {
		prev := current[len(current)-1]
		cur := lines[i]

		// prev is above cur on the page (prev.Y > cur.Y).
		// Gap = bottom of prev − top of cur.
		verticalGap := prev.Y - (cur.Y + cur.Height)
		lineSpacing := prev.Height * 1.5
		horizontalOverlap := overlaps(prev.X, prev.X+prev.Width, cur.X, cur.X+cur.Width)
		sameFontSize := math.Abs(prev.FontSize-cur.FontSize) < 1.0

		if verticalGap < lineSpacing && horizontalOverlap && sameFontSize {
			current = append(current, cur)
		} else {
			blocks = append(blocks, mergeLines(current, pageNum))
			current = []textLine{cur}
		}
	}

	if len(current) > 0 {
		blocks = append(blocks, mergeLines(current, pageNum))
	}

	return blocks
}

func mergeLines(lines []textLine, pageNum int) domain.TextBlock {
	if len(lines) == 0 {
		return domain.TextBlock{}
	}

	minX := lines[0].X
	minY := lines[0].Y
	maxX := lines[0].X + lines[0].Width
	maxY := lines[0].Y + lines[0].Height

	var texts []string
	var totalFontSize float64

	for _, l := range lines {
		texts = append(texts, l.Text)
		totalFontSize += l.FontSize

		if l.X < minX {
			minX = l.X
		}
		if l.Y < minY {
			minY = l.Y
		}
		if l.X+l.Width > maxX {
			maxX = l.X + l.Width
		}
		if l.Y+l.Height > maxY {
			maxY = l.Y + l.Height
		}
	}

	avgFontSize := totalFontSize / float64(len(lines))

	blockType := domain.BlockTypeText
	if avgFontSize > 16 {
		blockType = domain.BlockTypeTitle
	}

	return domain.TextBlock{
		PageNum: pageNum,
		BBox: domain.BoundingBox{
			X:      minX,
			Y:      minY,
			Width:  maxX - minX,
			Height: maxY - minY,
		},
		Text:      strings.Join(texts, "\n"),
		FontSize:  avgFontSize,
		FontName:  lines[0].FontName,
		BlockType: blockType,
	}
}

func overlaps(aMin, aMax, bMin, bMax float64) bool {
	overlapStart := math.Max(aMin, bMin)
	overlapEnd := math.Min(aMax, bMax)
	if overlapEnd <= overlapStart {
		return false
	}
	aWidth := aMax - aMin
	if aWidth == 0 {
		return false
	}
	overlapRatio := (overlapEnd - overlapStart) / aWidth
	return overlapRatio > 0.3
}
