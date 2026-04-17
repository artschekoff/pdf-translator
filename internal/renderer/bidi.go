package renderer

import "unicode"

// rtlRanges covers the primary RTL Unicode script blocks.
var rtlRanges = []*unicode.RangeTable{
	unicode.Arabic,
	unicode.Hebrew,
	unicode.Syriac,
	unicode.Thaana,
}

// IsRTL checks if the text is predominantly right-to-left.
func IsRTL(text string) bool {
	var rtlCount, ltrCount int
	for _, r := range text {
		if unicode.IsOneOf(rtlRanges, r) {
			rtlCount++
		} else if unicode.IsLetter(r) {
			ltrCount++
		}
	}
	return rtlCount > ltrCount
}

// scriptPriority defines the evaluation order for DetectScript.
// When two scripts tie for the highest count, the one appearing first wins,
// giving deterministic results regardless of map iteration order.
var scriptPriority = []string{
	"arabic",
	"hebrew",
	"devanagari",
	"cjk",
	"thai",
	"korean",
	"cyrillic",
	"latin",
}

// DetectScript returns the dominant script identifier for font selection.
func DetectScript(text string) string {
	scripts := make(map[string]int, len(scriptPriority))

	for _, r := range text {
		switch {
		case unicode.Is(unicode.Arabic, r):
			scripts["arabic"]++
		case unicode.Is(unicode.Hebrew, r):
			scripts["hebrew"]++
		case unicode.Is(unicode.Devanagari, r):
			scripts["devanagari"]++
		case unicode.Is(unicode.Han, r):
			scripts["cjk"]++
		case unicode.Is(unicode.Thai, r):
			scripts["thai"]++
		case unicode.Is(unicode.Hangul, r):
			scripts["korean"]++
		case unicode.Is(unicode.Cyrillic, r):
			scripts["cyrillic"]++
		case unicode.IsLetter(r):
			scripts["latin"]++
		}
	}

	maxCount := 0
	dominant := "latin"
	for _, script := range scriptPriority {
		if count := scripts[script]; count > maxCount {
			maxCount = count
			dominant = script
		}
	}

	return dominant
}
