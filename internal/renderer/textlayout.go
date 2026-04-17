package renderer

import (
	"strings"

	"github.com/pdf-translator/pdf-translator/internal/domain"
)

const (
	minFontScaleFactor    = 0.60
	maxBoxExpansion       = 1.50
	lineHeightFactor      = 1.35
	avgCharWidthRatioLat  = 0.50
	avgCharWidthRatioCJK  = 1.00
)

// LayoutResult holds the formatted text ready for rendering.
type LayoutResult struct {
	Lines      []string
	FontSize   float64
	LineHeight float64
	Overflow   bool
}

// FitText determines how to fit translated text within a bounding box.
// Algorithm: try original size -> word-wrap -> shrink font -> expand box -> truncate.
func FitText(text string, bbox domain.BoundingBox, originalFontSize float64) LayoutResult {
	if originalFontSize <= 0 {
		originalFontSize = 12
	}

	fontSize := originalFontSize
	charWidthRatio := charWidthRatioForText(text)

	lines := wrapText(text, bbox.Width, fontSize, charWidthRatio)
	lineHeight := fontSize * lineHeightFactor
	totalHeight := lineHeight * float64(len(lines))

	if totalHeight <= bbox.Height {
		return LayoutResult{
			Lines:      lines,
			FontSize:   fontSize,
			LineHeight: lineHeight,
		}
	}

	// Try shrinking font
	minFontSize := originalFontSize * minFontScaleFactor
	for fontSize >= minFontSize {
		fontSize -= 0.5
		lines = wrapText(text, bbox.Width, fontSize, charWidthRatio)
		lineHeight = fontSize * lineHeightFactor
		totalHeight = lineHeight * float64(len(lines))
		if totalHeight <= bbox.Height {
			return LayoutResult{
				Lines:      lines,
				FontSize:   fontSize,
				LineHeight: lineHeight,
			}
		}
	}

	// Try with expanded box
	expandedHeight := bbox.Height * maxBoxExpansion
	fontSize = originalFontSize * minFontScaleFactor
	lines = wrapText(text, bbox.Width, fontSize, charWidthRatio)
	lineHeight = fontSize * lineHeightFactor
	totalHeight = lineHeight * float64(len(lines))
	if totalHeight <= expandedHeight {
		return LayoutResult{
			Lines:      lines,
			FontSize:   fontSize,
			LineHeight: lineHeight,
			Overflow:   true,
		}
	}

	// Last resort: truncate
	maxLines := int(expandedHeight / lineHeight)
	if maxLines < 1 {
		maxLines = 1
	}
	if len(lines) > maxLines {
		lines = lines[:maxLines]
		runes := []rune(lines[maxLines-1])
		if len(runes) > 3 {
			lines[maxLines-1] = string(runes[:len(runes)-3]) + "..."
		}
	}

	return LayoutResult{
		Lines:      lines,
		FontSize:   fontSize,
		LineHeight: lineHeight,
		Overflow:   true,
	}
}

// charWidthRatioForText returns the character-width ratio for the dominant
// script in text. CJK characters are full-width (~1.0), Latin ~0.5.
func charWidthRatioForText(text string) float64 {
	switch DetectScript(text) {
	case "cjk", "korean":
		return avgCharWidthRatioCJK
	default:
		return avgCharWidthRatioLat
	}
}

// wrapText splits text into lines that fit within the given width at the given font size.
func wrapText(text string, maxWidth float64, fontSize float64, charWidthRatio float64) []string {
	charWidth := fontSize * charWidthRatio
	if charWidth <= 0 {
		charWidth = 6
	}
	charsPerLine := int(maxWidth / charWidth)
	if charsPerLine < 1 {
		charsPerLine = 1
	}

	paragraphs := strings.Split(text, "\n")
	var lines []string

	for _, para := range paragraphs {
		para = strings.TrimSpace(para)
		if para == "" {
			lines = append(lines, "")
			continue
		}

		words := strings.Fields(para)
		if len(words) == 0 {
			lines = append(lines, "")
			continue
		}

		currentLine := words[0]
		for _, word := range words[1:] {
			testLine := currentLine + " " + word
			if len([]rune(testLine)) <= charsPerLine {
				currentLine = testLine
			} else {
				lines = append(lines, currentLine)
				currentLine = word
			}
		}
		lines = append(lines, currentLine)
	}

	return lines
}

// MeasureTextWidth estimates the width of a string at a given font size.
func MeasureTextWidth(text string, fontSize float64) float64 {
	return float64(len([]rune(text))) * fontSize * charWidthRatioForText(text)
}
