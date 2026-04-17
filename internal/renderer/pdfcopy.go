package renderer

import (
	"strings"

	"github.com/razvandimescu/gopdf/pdf"
)

// pageCopier deep-copies PDF objects from a Reader into a Writer,
// remapping all indirect references.
type pageCopier struct {
	reader   *pdf.Reader
	writer   *pdf.Writer
	refCache map[int]pdf.Ref
}

func newPageCopier(reader *pdf.Reader, writer *pdf.Writer) *pageCopier {
	return &pageCopier{
		reader:   reader,
		writer:   writer,
		refCache: make(map[int]pdf.Ref),
	}
}

func (c *pageCopier) copyObject(obj any) any {
	switch v := obj.(type) {
	case pdf.Ref:
		if newRef, ok := c.refCache[v.Num]; ok {
			return newRef
		}
		newRef := c.writer.AllocRef()
		c.refCache[v.Num] = newRef

		resolved := c.reader.Resolve(v)
		if resolved == nil {
			c.writer.WriteObject(newRef, nil)
			return newRef
		}

		if stream, ok := resolved.(*pdf.Stream); ok {
			copiedDict := c.copyDict(stream.Dict)
			if isPassthroughFilter(stream.Dict) {
				copiedDict["Length"] = len(stream.Data)
				c.writer.WriteObject(newRef, &pdf.Stream{Dict: copiedDict, Data: stream.Data})
			} else {
				delete(copiedDict, "Filter")
				delete(copiedDict, "Length")
				delete(copiedDict, "DecodeParms")
				c.writer.WriteStream(newRef, copiedDict, stream.Data)
			}
			return newRef
		}

		copied := c.copyObject(resolved)
		c.writer.WriteObject(newRef, copied)
		return newRef

	case pdf.Dict:
		return c.copyDict(v)

	case pdf.Array:
		newArr := make(pdf.Array, len(v))
		for i, elem := range v {
			newArr[i] = c.copyObject(elem)
		}
		return newArr

	case pdf.Name, string, int, float64, bool, nil:
		return v

	default:
		return v
	}
}

func (c *pageCopier) copyDict(d pdf.Dict) pdf.Dict {
	newDict := make(pdf.Dict, len(d))
	for k, v := range d {
		if k == "Parent" {
			continue
		}
		newDict[k] = c.copyObject(v)
	}
	return newDict
}

// isPassthroughFilter returns true for filters whose data stays raw (images).
func isPassthroughFilter(d pdf.Dict) bool {
	var filters []pdf.Name
	if f, ok := d.Name("Filter"); ok {
		filters = []pdf.Name{f}
	} else if fa, ok := d.Array("Filter"); ok {
		for _, item := range fa {
			if n, ok := item.(pdf.Name); ok {
				filters = append(filters, n)
			}
		}
	}
	for _, f := range filters {
		switch f {
		case "DCTDecode", "JPXDecode", "CCITTFaxDecode", "JBIG2Decode":
			return true
		}
	}
	return false
}

// escapeStringPDF converts a Go (UTF-8) string to a PDF literal string in
// WinAnsiEncoding, escaping special characters. Type1 fonts like Helvetica
// use single-byte WinAnsiEncoding, not UTF-8.
func escapeStringPDF(s string) string {
	var b strings.Builder
	for _, r := range s {
		win := toWinAnsi(r)
		switch win {
		case '(', ')':
			b.WriteByte('\\')
			b.WriteByte(win)
		case '\\':
			b.WriteString("\\\\")
		case '\n':
			b.WriteString("\\n")
		case '\r':
			b.WriteString("\\r")
		case '\t':
			b.WriteString("\\t")
		default:
			b.WriteByte(win)
		}
	}
	return b.String()
}

// toWinAnsi maps a Unicode rune to its WinAnsiEncoding byte.
// Characters without a mapping are replaced with a sensible fallback.
func toWinAnsi(r rune) byte {
	// ASCII passthrough.
	if r >= 0x20 && r < 0x7F {
		return byte(r)
	}
	// Latin-1 Supplement (U+00A0–U+00FF) maps directly.
	if r >= 0xA0 && r <= 0xFF {
		return byte(r)
	}
	// WinAnsiEncoding special mappings (0x80–0x9F range).
	if b, ok := winAnsiSpecial[r]; ok {
		return b
	}
	// Bullet-like characters → • (0x95).
	switch r {
	case 0x25CF, 0x25CB, 0x25A0, 0x25AA, 0x2023, 0x25E6, 0x2043, 0x29BE:
		return 0x95
	}
	// Dash-like characters → – (0x96).
	switch r {
	case 0x2012, 0x2015:
		return 0x96
	}
	// Arrow / special list markers → -.
	if r >= 0x2190 && r <= 0x21FF {
		return '-'
	}
	// Non-breaking space variants.
	if r == 0x00A0 || r == 0x202F || r == 0x2007 || r == 0x2060 {
		return ' '
	}
	// Anything else that can't be encoded.
	return '?'
}

// winAnsiSpecial maps Unicode code points to their WinAnsiEncoding byte
// positions in the 0x80–0x9F range.
var winAnsiSpecial = map[rune]byte{
	0x20AC: 0x80, // €
	0x201A: 0x82, // ‚
	0x0192: 0x83, // ƒ
	0x201E: 0x84, // „
	0x2026: 0x85, // …
	0x2020: 0x86, // †
	0x2021: 0x87, // ‡
	0x02C6: 0x88, // ˆ
	0x2030: 0x89, // ‰
	0x0160: 0x8A, // Š
	0x2039: 0x8B, // ‹
	0x0152: 0x8C, // Œ
	0x017D: 0x8E, // Ž
	0x2018: 0x91, // '
	0x2019: 0x92, // '
	0x201C: 0x93, // "
	0x201D: 0x94, // "
	0x2022: 0x95, // •
	0x2013: 0x96, // –
	0x2014: 0x97, // —
	0x02DC: 0x98, // ˜
	0x2122: 0x99, // ™
	0x0161: 0x9A, // š
	0x203A: 0x9B, // ›
	0x0153: 0x9C, // œ
	0x017E: 0x9E, // ž
	0x0178: 0x9F, // Ÿ
}
