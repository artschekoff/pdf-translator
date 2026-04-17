package renderer

import (
	"image"
	"image/color"

	"github.com/pdf-translator/pdf-translator/internal/domain"
)

// DetectBackgroundColor samples the border pixels of a bounding box region
// in the page image and returns the dominant color.
func DetectBackgroundColor(img image.Image, bbox domain.BoundingBox, dpi int) (r, g, b float64) {
	if img == nil {
		return 1, 1, 1 // white fallback
	}

	pxX := int(bbox.X * float64(dpi) / 72.0)
	pxY := int(bbox.Y * float64(dpi) / 72.0)
	pxW := int(bbox.Width * float64(dpi) / 72.0)
	pxH := int(bbox.Height * float64(dpi) / 72.0)

	bounds := img.Bounds()
	pxX = clampInt(pxX, bounds.Min.X, bounds.Max.X-1)
	pxY = clampInt(pxY, bounds.Min.Y, bounds.Max.Y-1)

	var totalR, totalG, totalB uint64
	var count uint64

	if pxW <= 0 || pxH <= 0 {
		return 1, 1, 1
	}

	// Sample border pixels (top, bottom, left, right edges of the box)
	bottomY := clampInt(pxY+pxH-1, bounds.Min.Y, bounds.Max.Y-1)
	rightX := clampInt(pxX+pxW-1, bounds.Min.X, bounds.Max.X-1)
	for x := pxX; x < pxX+pxW && x < bounds.Max.X; x++ {
		addPixel(img, x, pxY, &totalR, &totalG, &totalB, &count)
		addPixel(img, x, bottomY, &totalR, &totalG, &totalB, &count)
	}
	for y := pxY; y < pxY+pxH && y < bounds.Max.Y; y++ {
		addPixel(img, pxX, y, &totalR, &totalG, &totalB, &count)
		addPixel(img, rightX, y, &totalR, &totalG, &totalB, &count)
	}

	if count == 0 {
		return 1, 1, 1
	}

	r = float64(totalR/count) / 65535.0
	g = float64(totalG/count) / 65535.0
	b = float64(totalB/count) / 65535.0

	return r, g, b
}

func addPixel(img image.Image, x, y int, totalR, totalG, totalB, count *uint64) {
	c := img.At(x, y)
	rr, gg, bb, _ := c.RGBA()
	*totalR += uint64(rr)
	*totalG += uint64(gg)
	*totalB += uint64(bb)
	*count++
}

func clampInt(val, minVal, maxVal int) int {
	if val < minVal {
		return minVal
	}
	if val > maxVal {
		return maxVal
	}
	return val
}

// IsWhiteBackground checks if the detected color is close to white.
func IsWhiteBackground(r, g, b float64) bool {
	threshold := 0.95
	return r > threshold && g > threshold && b > threshold
}

// ColorToUint8 converts normalized float64 color channels to uint8.
func ColorToUint8(r, g, b float64) color.RGBA {
	return color.RGBA{
		R: uint8(r * 255),
		G: uint8(g * 255),
		B: uint8(b * 255),
		A: 255,
	}
}
