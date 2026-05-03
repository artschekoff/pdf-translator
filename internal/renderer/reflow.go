package renderer

import (
	"sort"
	"strings"

	"github.com/pdf-translator/pdf-translator/internal/domain"
)

const (
	// Vertical gap (pts) between PARAGRAPH groups that signals a diagram region.
	// Only paragraph-width blocks participate in gap detection; narrow label
	// blocks (diagram annotations) are assigned to whichever segment their
	// y-centre falls in.
	diagramGapThreshold = 60.0

	// A block whose width exceeds this fraction of the page width is treated as
	// "paragraph" text.  Narrower blocks are diagram labels / captions.
	paragraphWidthFraction = 0.45

	// Visual gap inserted between the "original" and "translated" views of a
	// diagram segment in the re-flowed output.
	diagramSectionGap = 10.0

	// Short lines that sit immediately above/below a paragraph group are often
	// the trailing line of the same paragraph, not diagram text.
	paragraphAttachmentGap       = 18.0
	paragraphAttachmentMaxHeight = 18.0
	paragraphContainedGap        = 40.0
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

type paragraphGroup struct {
	yLow, yHigh float64
	xLow, xHigh float64
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

// buildSegments partitions the page into alternating text / diagram strips.
//
// Key insight: gap detection uses only PARAGRAPH blocks (width > 45 % of
// page width).  Narrow diagram-label blocks (e.g. "Hash", "Block", "Sign")
// do NOT participate in gap detection — they are simply assigned to the
// segment whose y-range contains their y-centre.  This prevents diagram
// labels from being "absorbed" into the paragraph group and shrinking the
// detected diagram region.
//
// For each pair of adjacent paragraph groups separated by a gap ≥
// diagramGapThreshold, a diagram segment is created that spans the full gap.
func buildSegments(blocks []domain.TextBlock, pageH, pageW float64) []pageSegment {
	if len(blocks) == 0 {
		return nil
	}

	minParagraphW := pageW * paragraphWidthFraction

	// Separate into paragraph blocks (wide) and label blocks (narrow).
	var paragraphs []domain.TextBlock
	for _, b := range blocks {
		if domain.IsTextualBlockType(b.BlockType) && strings.TrimSpace(b.Text) != "" && b.BBox.Width >= minParagraphW {
			paragraphs = append(paragraphs, b)
		}
	}

	// If there are no paragraph-width blocks, treat everything as a diagram
	// (e.g. a page that is entirely a figure with captions).
	if len(paragraphs) == 0 {
		seg := pageSegment{isDiagram: true, yLow: 0, yHigh: pageH, renderLow: 0, renderHigh: pageH, blocks: blocks}
		return []pageSegment{seg}
	}

	// Sort paragraphs top → bottom (high y first).
	sort.Slice(paragraphs, func(i, j int) bool {
		return top(paragraphs[i]) > top(paragraphs[j])
	})

	// Merge consecutive paragraphs into groups.
	cur := paragraphGroup{
		yLow:  paragraphs[0].BBox.Y,
		yHigh: top(paragraphs[0]),
		xLow:  paragraphs[0].BBox.X,
		xHigh: paragraphs[0].BBox.X + paragraphs[0].BBox.Width,
	}
	var groups []paragraphGroup
	for _, p := range paragraphs[1:] {
		pTop := top(p)
		gap := cur.yLow - pTop
		if gap < 0 {
			gap = 0
		}
		if gap <= diagramGapThreshold {
			if p.BBox.Y < cur.yLow {
				cur.yLow = p.BBox.Y
			}
			if p.BBox.X < cur.xLow {
				cur.xLow = p.BBox.X
			}
			if xHigh := p.BBox.X + p.BBox.Width; xHigh > cur.xHigh {
				cur.xHigh = xHigh
			}
		} else {
			groups = append(groups, cur)
			cur = paragraphGroup{
				yLow:  p.BBox.Y,
				yHigh: pTop,
				xLow:  p.BBox.X,
				xHigh: p.BBox.X + p.BBox.Width,
			}
		}
	}
	groups = append(groups, cur)

	// Expand each paragraph group to absorb short adjacent lines that belong to
	// the same paragraph but were excluded from gap detection because they are
	// much narrower than the main paragraph body.
	for changed := true; changed; {
		changed = false
		for _, b := range blocks {
			if b.BBox.Width >= minParagraphW {
				continue
			}

			bestIdx := -1
			bestGap := 0.0
			for i := range groups {
				gap, ok := attachmentGap(groups[i], b)
				if !ok {
					continue
				}
				if bestIdx == -1 || gap < bestGap {
					bestIdx = i
					bestGap = gap
				}
			}

			if bestIdx == -1 {
				continue
			}

			if expandGroup(&groups[bestIdx], b) {
				changed = true
			}
		}
	}

	// Build segments: text groups + diagram gaps between them.
	// We do NOT add top/bottom margin segments (page numbers, headers, footers).
	var segs []pageSegment
	for i, g := range groups {
		segs = append(segs, pageSegment{isDiagram: false, yLow: g.yLow, yHigh: g.yHigh, renderLow: g.yLow, renderHigh: g.yHigh})
		if i+1 < len(groups) {
			nextG := groups[i+1]
			gap := g.yLow - nextG.yHigh
			if gap >= diagramGapThreshold {
				segs = append(segs, pageSegment{isDiagram: true, yLow: nextG.yHigh, yHigh: g.yLow, renderLow: nextG.yHigh, renderHigh: g.yLow})
			}
		}
	}

	// Assign every text block to a segment.
	//
	// Strategy (two-pass):
	//  1. Try TEXT segments first using bounding-box overlap. This catches
	//     "last line of paragraph" blocks whose y-centre has drifted just
	//     below the paragraph group boundary into the diagram gap.
	//  2. Try DIAGRAM segments by y-centre for true diagram labels.
	//  3. Fall back to nearest segment by centre distance.
	for _, b := range blocks {
		bLow := b.BBox.Y
		bHigh := b.BBox.Y + b.BBox.Height
		centre := (bLow + bHigh) / 2

		assigned := false

		// Pass 1: text segments, overlap test.
		for i := range segs {
			if segs[i].isDiagram {
				continue
			}
			if bLow < segs[i].yHigh && bHigh > segs[i].yLow {
				segs[i].blocks = append(segs[i].blocks, b)
				assigned = true
				break
			}
		}

		// Pass 2: diagram segments, y-centre test.
		if !assigned {
			for i := range segs {
				if !segs[i].isDiagram {
					continue
				}
				if centre >= segs[i].yLow && centre <= segs[i].yHigh {
					segs[i].blocks = append(segs[i].blocks, b)
					assigned = true
					break
				}
			}
		}

		// Pass 3: nearest segment by centre distance.
		if !assigned {
			best := 0
			bestDist := 1e18
			for i := range segs {
				mid := (segs[i].yLow + segs[i].yHigh) / 2
				d := centre - mid
				if d < 0 {
					d = -d
				}
				if d < bestDist {
					bestDist = d
					best = i
				}
			}
			segs[best].blocks = append(segs[best].blocks, b)
		}
	}

	refineDiagramRenderBounds(segs, pageH)
	return segs
}

func top(b domain.TextBlock) float64 { return b.BBox.Y + b.BBox.Height }

func attachmentGap(g paragraphGroup, b domain.TextBlock) (float64, bool) {
	bLow := b.BBox.Y
	bHigh := top(b)
	overlap := horizontalOverlapFraction(g.xLow, g.xHigh, b.BBox.X, b.BBox.X+b.BBox.Width)
	if overlap == 0 {
		return 0, false
	}
	maxGap := paragraphAttachmentGap
	groupWidth := g.xHigh - g.xLow
	containedNarrow := groupWidth > 0 && overlap == 1 && b.BBox.Width <= groupWidth*0.5
	if !containedNarrow && b.BBox.Height > paragraphAttachmentMaxHeight {
		return 0, false
	}
	if containedNarrow {
		maxGap = paragraphContainedGap
	}

	switch {
	case bHigh <= g.yLow:
		gap := g.yLow - bHigh
		return gap, gap <= maxGap
	case bLow >= g.yHigh:
		gap := bLow - g.yHigh
		return gap, gap <= maxGap
	case bLow < g.yHigh && bHigh > g.yLow:
		return 0, true
	default:
		return 0, false
	}
}

func expandGroup(g *paragraphGroup, b domain.TextBlock) bool {
	changed := false
	if b.BBox.Y < g.yLow {
		g.yLow = b.BBox.Y
		changed = true
	}
	if bHigh := top(b); bHigh > g.yHigh {
		g.yHigh = bHigh
		changed = true
	}
	if b.BBox.X < g.xLow {
		g.xLow = b.BBox.X
		changed = true
	}
	if bHigh := b.BBox.X + b.BBox.Width; bHigh > g.xHigh {
		g.xHigh = bHigh
		changed = true
	}
	return changed
}

func horizontalOverlapFraction(aLow, aHigh, bLow, bHigh float64) float64 {
	low := maxFloat(aLow, bLow)
	high := minFloat(aHigh, bHigh)
	if high <= low {
		return 0
	}
	width := bHigh - bLow
	if width <= 0 {
		return 0
	}
	return (high - low) / width
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxFloat(a, b float64) float64 {
	if a > b {
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
