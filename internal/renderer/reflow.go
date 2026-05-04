package renderer

import (
	"sort"
	"strings"

	"github.com/pdf-translator/pdf-translator/internal/domain"
)

const (
	// Visual gap inserted between the "original" and "translated" views of a
	// diagram segment in the re-flowed output.
	diagramSectionGap = 10.0
)

// pageSegment is a horizontal strip of a page — either a text region or a
// graphical/diagram region.
type pageSegment struct {
	isDiagram  bool
	yLow       float64 // original page coords — bottom of strip
	yHigh      float64 // original page coords — top of strip
	renderLow  float64 // actual rendered/clip bounds
	renderHigh float64
	blocks     []domain.TextBlock // text blocks assigned to this segment
}

func (s pageSegment) height() float64 { return s.renderHigh - s.renderLow }

// outputHeight returns the vertical space this segment occupies in the
// re-flowed output.  Diagram segments appear twice (original + translated).
func (s pageSegment) outputHeight() float64 {
	h := s.height()
	if s.isDiagram {
		return h*2 + diagramSectionGap
	}
	return h
}

// buildSegments partitions the page into text and diagram strips using
// PaddleOCR's layout classification. Blocks with BlockTypeImage are diagram
// regions; all other blocks are text regions.
func buildSegments(blocks []domain.TextBlock, pageH, pageW float64) []pageSegment {
	if len(blocks) == 0 {
		return nil
	}

	// Collect image-type blocks (PaddleOCR: figure, chart, image, formula, seal).
	var imageBlocks []domain.TextBlock
	for _, b := range blocks {
		if !domain.IsTextualBlockType(b.BlockType) {
			imageBlocks = append(imageBlocks, b)
		}
	}

	// No image blocks: entire page is a single text segment.
	if len(imageBlocks) == 0 {
		var textBlocks []domain.TextBlock
		yLow, yHigh := pageH, 0.0
		for _, b := range blocks {
			if strings.TrimSpace(b.Text) == "" {
				continue
			}
			textBlocks = append(textBlocks, b)
			if b.BBox.Y < yLow {
				yLow = b.BBox.Y
			}
			if t := top(b); t > yHigh {
				yHigh = t
			}
		}
		if len(textBlocks) == 0 {
			return nil
		}
		return []pageSegment{{
			isDiagram:  false,
			yLow:       yLow,
			yHigh:      yHigh,
			renderLow:  yLow,
			renderHigh: yHigh,
			blocks:     textBlocks,
		}}
	}

	// Sort image blocks top → bottom (higher y = higher on page in PDF coords).
	sort.Slice(imageBlocks, func(i, j int) bool {
		return top(imageBlocks[i]) > top(imageBlocks[j])
	})

	// Merge overlapping image blocks into diagram regions.
	type region struct{ lo, hi float64 }
	regions := []region{{lo: imageBlocks[0].BBox.Y, hi: top(imageBlocks[0])}}
	for _, b := range imageBlocks[1:] {
		lo, hi := b.BBox.Y, top(b)
		last := &regions[len(regions)-1]
		if lo < last.hi && last.lo < hi { // regions overlap
			if lo < last.lo {
				last.lo = lo
			}
			if hi > last.hi {
				last.hi = hi
			}
		} else {
			regions = append(regions, region{lo, hi})
		}
	}
	// regions sorted top→bottom: regions[0].hi is highest y (top of page).

	// Build alternating text / diagram segments between image region boundaries.
	var segs []pageSegment
	prevTop := pageH
	for _, r := range regions {
		if prevTop > r.hi {
			segs = append(segs, pageSegment{
				isDiagram:  false,
				yLow:       r.hi,
				yHigh:      prevTop,
				renderLow:  r.hi,
				renderHigh: prevTop,
			})
		}
		segs = append(segs, pageSegment{
			isDiagram:  true,
			yLow:       r.lo,
			yHigh:      r.hi,
			renderLow:  r.lo,
			renderHigh: r.hi,
		})
		prevTop = r.lo
	}
	if prevTop > 0 {
		segs = append(segs, pageSegment{
			isDiagram:  false,
			yLow:       0,
			yHigh:      prevTop,
			renderLow:  0,
			renderHigh: prevTop,
		})
	}

	// Assign each block to the nearest segment.
	// Image blocks are restricted to diagram segments; text blocks go anywhere.
	for _, b := range blocks {
		centre := b.BBox.Y + b.BBox.Height/2
		isImage := !domain.IsTextualBlockType(b.BlockType)

		bestIdx := -1
		bestDist := 1e18
		for i := range segs {
			if isImage && !segs[i].isDiagram {
				continue
			}
			mid := (segs[i].yLow + segs[i].yHigh) / 2
			if d := abs64(centre - mid); d < bestDist {
				bestDist = d
				bestIdx = i
			}
		}
		if bestIdx >= 0 {
			segs[bestIdx].blocks = append(segs[bestIdx].blocks, b)
		}
	}

	refineDiagramRenderBounds(segs, pageH)
	return segs
}

func top(b domain.TextBlock) float64 { return b.BBox.Y + b.BBox.Height }

func abs64(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func refineDiagramRenderBounds(segs []pageSegment, pageH float64) {
	for i := range segs {
		segs[i].renderLow = segs[i].yLow
		segs[i].renderHigh = segs[i].yHigh
		if !segs[i].isDiagram || len(segs[i].blocks) == 0 {
			continue
		}

		pad := diagramPadding(segs[i].blocks)
		low := segs[i].yLow - pad
		high := segs[i].yHigh + pad

		if i > 0 && !segs[i-1].isDiagram {
			prevLimit := segs[i-1].yLow + minFloat(pad*0.5, 8)
			if high > prevLimit {
				high = prevLimit
			}
		}
		if i+1 < len(segs) && !segs[i+1].isDiagram {
			nextLimit := segs[i+1].yHigh - minFloat(pad*0.5, 8)
			if low < nextLimit {
				low = nextLimit
			}
		}

		if low < 0 {
			low = 0
		}
		if high > pageH {
			high = pageH
		}
		if high <= low {
			low = segs[i].yLow
			high = segs[i].yHigh
		}
		segs[i].renderLow = low
		segs[i].renderHigh = high
	}
}

func diagramPadding(blocks []domain.TextBlock) float64 {
	maxH := 0.0
	minH := 1e18
	for _, b := range blocks {
		if b.BBox.Height > maxH {
			maxH = b.BBox.Height
		}
		if b.BBox.Height < minH {
			minH = b.BBox.Height
		}
	}
	if maxH == 0 {
		return 12
	}

	pad := maxH * 1.75
	if minH > 0 && minH*2 > pad {
		pad = minH * 2
	}
	if pad < 12 {
		pad = 12
	}
	if pad > 28 {
		pad = 28
	}
	return pad
}
