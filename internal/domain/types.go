package domain

// BoundingBox represents a rectangular region on a PDF page in PDF points.
type BoundingBox struct {
	X      float64
	Y      float64
	Width  float64
	Height float64
}

// TextBlock is an extracted text region with position and metadata.
type TextBlock struct {
	PageNum    int
	BBox       BoundingBox
	Text       string
	Translated string
	FontSize   float64
	FontName   string
	BlockType  string // "text", "title", "header", "footer", "table", etc.
}

// Page holds all extracted text blocks for a single PDF page.
type Page struct {
	Number int
	Width  float64
	Height float64
	Blocks []TextBlock
}

// PageType indicates whether a page contains selectable text or is image-based.
type PageType string

const (
	PageTypeNative  PageType = "native"
	PageTypeScanned PageType = "scanned"
)

// BlockType constants for text block classification.
const (
	BlockTypeText    = "text"
	BlockTypeTitle   = "title"
	BlockTypeHeader  = "header"
	BlockTypeFooter  = "footer"
	BlockTypeCaption = "caption"
	BlockTypeTable   = "table"
	BlockTypeImage   = "image"
)

// IsTextualBlockType reports whether a block type should carry translatable text.
func IsTextualBlockType(blockType string) bool {
	return blockType != BlockTypeImage
}

// Script identifiers used for font selection.
const (
	ScriptLatin    = "latin"
	ScriptCyrillic = "cyrillic"
	ScriptArabic   = "arabic"
	ScriptCJK      = "cjk"
)

// OCR engine identifiers.
const (
	OCREnginePaddleOCR = "paddleocr"
	OCREngineTesseract = "tesseract"
	OCREngineDocling   = "docling"
)

// TranslateRequest carries all parameters for a single translation job.
type TranslateRequest struct {
	InputPath  string
	OutputPath string
	SourceLang string
	TargetLang string
	OCREngine  string
	Password   string
	Pages      string // e.g. "1-5" or "" for all
	Workers    int
	DPI        int
	DryRun         bool
	KeepOriginal   bool
	TranslateColor string // text color for translated paragraphs in keep-original mode
}
