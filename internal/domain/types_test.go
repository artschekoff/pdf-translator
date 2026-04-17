package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPageTypeConstants(t *testing.T) {
	assert.Equal(t, PageType("native"), PageTypeNative)
	assert.Equal(t, PageType("scanned"), PageTypeScanned)
}

func TestBoundingBox(t *testing.T) {
	bbox := BoundingBox{X: 72, Y: 700, Width: 200, Height: 50}
	assert.Equal(t, 72.0, bbox.X)
	assert.Equal(t, 700.0, bbox.Y)
	assert.Equal(t, 200.0, bbox.Width)
	assert.Equal(t, 50.0, bbox.Height)
}
