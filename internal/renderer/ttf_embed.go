package renderer

import (
	"encoding/binary"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/razvandimescu/gopdf/pdf"
)

// TTFFont holds the parsed data from a TrueType font needed for PDF embedding.
type TTFFont struct {
	data     []byte
	name     string
	glyphMap map[rune]uint16 // Unicode codepoint → glyph ID

	unitsPerEm  uint16
	xMin, yMin  int16
	xMax, yMax  int16
	ascender    int16
	descender   int16
	capHeight   int16
	glyphWidths []uint16 // indexed by GID → advance width in font units
}

// LoadTTFFont reads and parses a TrueType font file.
func LoadTTFFont(path string) (*TTFFont, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading font: %w", err)
	}
	return parseTTFFont(data)
}

func parseTTFFont(data []byte) (*TTFFont, error) {
	if len(data) < 12 {
		return nil, fmt.Errorf("not a valid TTF file")
	}
	f := &TTFFont{data: data, unitsPerEm: 1000}

	tables, err := parseTTFTables(data)
	if err != nil {
		return nil, err
	}

	cmapData, ok := tables["cmap"]
	if !ok {
		return nil, fmt.Errorf("font has no cmap table")
	}
	gm, err := parseCmapTable(cmapData)
	if err != nil {
		return nil, fmt.Errorf("parsing cmap: %w", err)
	}
	f.glyphMap = gm

	if d, ok := tables["head"]; ok && len(d) >= 44 {
		f.unitsPerEm = binary.BigEndian.Uint16(d[18:])
		f.xMin = int16(binary.BigEndian.Uint16(d[36:]))
		f.yMin = int16(binary.BigEndian.Uint16(d[38:]))
		f.xMax = int16(binary.BigEndian.Uint16(d[40:]))
		f.yMax = int16(binary.BigEndian.Uint16(d[42:]))
	}
	if d, ok := tables["hhea"]; ok && len(d) >= 8 {
		f.ascender = int16(binary.BigEndian.Uint16(d[4:]))
		f.descender = int16(binary.BigEndian.Uint16(d[6:]))
	}
	if d, ok := tables["OS/2"]; ok && len(d) >= 90 {
		f.capHeight = int16(binary.BigEndian.Uint16(d[88:]))
	}
	if d, ok := tables["name"]; ok {
		f.name = parsePSFontName(d)
	}
	if f.name == "" {
		f.name = "EmbeddedFont"
	}

	// Parse glyph advance widths from hmtx (requires numberOfHMetrics from hhea
	// and numGlyphs from maxp).
	var numHMetrics, numGlyphs int
	if d, ok := tables["hhea"]; ok && len(d) >= 36 {
		numHMetrics = int(binary.BigEndian.Uint16(d[34:]))
	}
	if d, ok := tables["maxp"]; ok && len(d) >= 6 {
		numGlyphs = int(binary.BigEndian.Uint16(d[4:]))
	}
	if hmtx, ok := tables["hmtx"]; ok && numHMetrics > 0 && numGlyphs > 0 {
		f.glyphWidths = parseHmtx(hmtx, numHMetrics, numGlyphs)
	}

	return f, nil
}

func parseTTFTables(data []byte) (map[string][]byte, error) {
	numTables := int(binary.BigEndian.Uint16(data[4:]))
	tables := make(map[string][]byte, numTables)
	for i := range numTables {
		rec := data[12+i*16:]
		if len(rec) < 16 {
			break
		}
		tag := string(rec[:4])
		offset := int(binary.BigEndian.Uint32(rec[8:]))
		length := int(binary.BigEndian.Uint32(rec[12:]))
		if offset+length <= len(data) {
			tables[tag] = data[offset : offset+length]
		}
	}
	return tables, nil
}

func parseCmapTable(data []byte) (map[rune]uint16, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("cmap too short")
	}
	n := int(binary.BigEndian.Uint16(data[2:]))

	type candidate struct {
		data  []byte
		score int
	}
	var best candidate

	for i := range n {
		off := 4 + i*8
		if off+8 > len(data) {
			break
		}
		platform := binary.BigEndian.Uint16(data[off:])
		encoding := binary.BigEndian.Uint16(data[off+2:])
		subtableOff := int(binary.BigEndian.Uint32(data[off+4:]))
		if subtableOff+4 > len(data) {
			continue
		}
		sub := data[subtableOff:]
		if len(sub) < 2 {
			continue
		}
		format := binary.BigEndian.Uint16(sub[:2])

		var score int
		switch {
		case platform == 3 && encoding == 10 && format == 12:
			score = 3
		case platform == 3 && encoding == 1 && format == 4:
			score = 2
		case platform == 0 && encoding == 3 && format == 4:
			score = 1
		}
		if score > best.score {
			best = candidate{sub, score}
		}
	}

	if best.data == nil {
		return nil, fmt.Errorf("no usable cmap subtable")
	}
	switch binary.BigEndian.Uint16(best.data[:2]) {
	case 4:
		return parseCmapFmt4(best.data)
	case 12:
		return parseCmapFmt12(best.data)
	default:
		return nil, fmt.Errorf("unsupported cmap format")
	}
}

func parseCmapFmt4(data []byte) (map[rune]uint16, error) {
	if len(data) < 14 {
		return nil, fmt.Errorf("cmap format 4 too short")
	}
	segCount := int(binary.BigEndian.Uint16(data[6:])) / 2

	endBase := 14
	startBase := endBase + segCount*2 + 2 // +2 for reserved pad
	deltaBase := startBase + segCount*2
	rangeBase := deltaBase + segCount*2

	if rangeBase+segCount*2 > len(data) {
		return nil, fmt.Errorf("cmap format 4 truncated")
	}

	m := make(map[rune]uint16, segCount*16)
	for i := range segCount {
		end := int(binary.BigEndian.Uint16(data[endBase+i*2:]))
		start := int(binary.BigEndian.Uint16(data[startBase+i*2:]))
		delta := int(int16(binary.BigEndian.Uint16(data[deltaBase+i*2:])))
		rangeOff := int(binary.BigEndian.Uint16(data[rangeBase+i*2:]))

		if start == 0xFFFF {
			break
		}
		for c := start; c <= end; c++ {
			var gid uint16
			if rangeOff == 0 {
				gid = uint16((c + delta) & 0xFFFF)
			} else {
				idx := rangeBase + i*2 + rangeOff + (c-start)*2
				if idx+2 > len(data) {
					continue
				}
				g := int(binary.BigEndian.Uint16(data[idx:]))
				if g == 0 {
					continue
				}
				gid = uint16((g + delta) & 0xFFFF)
			}
			if gid != 0 {
				m[rune(c)] = gid
			}
		}
	}
	return m, nil
}

func parseCmapFmt12(data []byte) (map[rune]uint16, error) {
	if len(data) < 16 {
		return nil, fmt.Errorf("cmap format 12 too short")
	}
	numGroups := int(binary.BigEndian.Uint32(data[12:]))
	m := make(map[rune]uint16, numGroups*10)
	for i := range numGroups {
		off := 16 + i*12
		if off+12 > len(data) {
			break
		}
		startCode := int(binary.BigEndian.Uint32(data[off:]))
		endCode := int(binary.BigEndian.Uint32(data[off+4:]))
		startGlyph := int(binary.BigEndian.Uint32(data[off+8:]))
		for c := startCode; c <= endCode && c <= 0xFFFF; c++ {
			gid := uint16(startGlyph + c - startCode)
			if gid != 0 {
				m[rune(c)] = gid
			}
		}
	}
	return m, nil
}

func parsePSFontName(data []byte) string {
	if len(data) < 6 {
		return ""
	}
	count := int(binary.BigEndian.Uint16(data[2:]))
	storageOff := int(binary.BigEndian.Uint16(data[4:]))

	for i := range count {
		off := 6 + i*12
		if off+12 > len(data) {
			break
		}
		platform := binary.BigEndian.Uint16(data[off:])
		nameID := binary.BigEndian.Uint16(data[off+6:])
		if nameID != 6 { // PostScript name
			continue
		}
		length := int(binary.BigEndian.Uint16(data[off+8:]))
		strOff := int(binary.BigEndian.Uint16(data[off+10:]))
		start := storageOff + strOff
		if start+length > len(data) {
			continue
		}
		b := data[start : start+length]
		if platform == 3 { // Windows UTF-16BE
			var sb strings.Builder
			for j := 0; j+1 < len(b); j += 2 {
				r := rune(binary.BigEndian.Uint16(b[j:]))
				if r < 128 {
					sb.WriteRune(r)
				}
			}
			if sb.Len() > 0 {
				return sb.String()
			}
		} else {
			return string(b)
		}
	}
	return ""
}

func parseHmtx(data []byte, numHMetrics, numGlyphs int) []uint16 {
	widths := make([]uint16, numGlyphs)
	var lastWidth uint16
	for i := range numHMetrics {
		if i*4+2 > len(data) {
			break
		}
		w := binary.BigEndian.Uint16(data[i*4:])
		if int(i) < numGlyphs {
			widths[i] = w
		}
		lastWidth = w
	}
	for i := numHMetrics; i < numGlyphs; i++ {
		widths[i] = lastWidth
	}
	return widths
}

// buildWidthsPDF returns a PDF /W array for the CIDFont dict, grouping mapped
// GIDs into consecutive runs so the viewer uses real per-glyph advance widths.
func (f *TTFFont) buildWidthsPDF() pdf.Array {
	if len(f.glyphWidths) == 0 {
		return nil
	}

	// Collect unique GIDs present in glyphMap.
	seen := make(map[uint16]bool, len(f.glyphMap))
	for _, gid := range f.glyphMap {
		seen[gid] = true
	}
	gids := make([]int, 0, len(seen))
	for gid := range seen {
		gids = append(gids, int(gid))
	}
	sort.Ints(gids)

	result := make(pdf.Array, 0, len(gids)*2)
	i := 0
	for i < len(gids) {
		runStart := gids[i]
		widths := make(pdf.Array, 0, 8)
		for i < len(gids) && gids[i] == runStart+len(widths) {
			gid := uint16(gids[i])
			var w int
			if int(gid) < len(f.glyphWidths) {
				w = int(float64(f.glyphWidths[gid]) * 1000.0 / float64(f.unitsPerEm))
			} else {
				w = 1000
			}
			widths = append(widths, w)
			i++
		}
		result = append(result, runStart, widths)
	}
	return result
}

// HasGlyph reports whether the font contains a glyph for the given rune.
func (f *TTFFont) HasGlyph(r rune) bool {
	_, ok := f.glyphMap[r]
	return ok
}

// EncodeTextHex converts a Unicode string to uppercase hex-encoded GlyphIDs
// for use as a PDF <hex-string> with a Type0 Identity-H encoded font.
func (f *TTFFont) EncodeTextHex(text string) string {
	var b strings.Builder
	for _, r := range text {
		gid := f.glyphMap[r] // 0 = .notdef for unmapped runes
		fmt.Fprintf(&b, "%04X", gid)
	}
	return b.String()
}

func (f *TTFFont) scaledInt(v int16) int {
	if f.unitsPerEm == 0 {
		return int(v)
	}
	return int(float64(v) * 1000.0 / float64(f.unitsPerEm))
}

// EmbedInPDF writes the font objects to w and returns an indirect Ref to the
// Type0 font dictionary. Put this Ref in the page Resources /Font dict.
func (f *TTFFont) EmbedInPDF(w *pdf.Writer) (pdf.Ref, error) {
	// FontFile2 stream (raw TTF bytes).
	fontFileRef := w.AllocRef()
	if err := w.WriteStream(fontFileRef, pdf.Dict{"Length1": len(f.data)}, f.data); err != nil {
		return pdf.Ref{}, fmt.Errorf("writing font stream: %w", err)
	}

	// FontDescriptor.
	fdRef := w.AllocRef()
	if err := w.WriteObject(fdRef, pdf.Dict{
		"Type":        pdf.Name("FontDescriptor"),
		"FontName":    pdf.Name(f.name),
		"Flags":       int(32), // Nonsymbolic
		"FontBBox":    pdf.Array{f.scaledInt(f.xMin), f.scaledInt(f.yMin), f.scaledInt(f.xMax), f.scaledInt(f.yMax)},
		"ItalicAngle": int(0),
		"Ascent":      f.scaledInt(f.ascender),
		"Descent":     f.scaledInt(f.descender),
		"CapHeight":   f.scaledInt(f.capHeight),
		"StemV":       int(80),
		"FontFile2":   fontFileRef,
	}); err != nil {
		return pdf.Ref{}, fmt.Errorf("writing font descriptor: %w", err)
	}

	// ToUnicode CMap (enables copy-paste of text from the resulting PDF).
	toUnicodeRef := w.AllocRef()
	if err := w.WriteStream(toUnicodeRef, pdf.Dict{}, buildToUnicodeCMap(f.name, f.glyphMap)); err != nil {
		return pdf.Ref{}, fmt.Errorf("writing ToUnicode CMap: %w", err)
	}

	// Descendant CIDFont.
	cidDict := pdf.Dict{
		"Type":    pdf.Name("Font"),
		"Subtype": pdf.Name("CIDFontType2"),
		"BaseFont": pdf.Name(f.name),
		"CIDSystemInfo": pdf.Dict{
			"Registry":   "Adobe",
			"Ordering":   "Identity",
			"Supplement": int(0),
		},
		"FontDescriptor": fdRef,
		"DW":             int(500),
		"CIDToGIDMap":    pdf.Name("Identity"),
	}
	if wArr := f.buildWidthsPDF(); len(wArr) > 0 {
		cidDict["W"] = wArr
	}
	cidRef := w.AllocRef()
	if err := w.WriteObject(cidRef, cidDict); err != nil {
		return pdf.Ref{}, fmt.Errorf("writing CIDFont: %w", err)
	}

	// Type0 wrapper font.
	type0Ref := w.AllocRef()
	if err := w.WriteObject(type0Ref, pdf.Dict{
		"Type":            pdf.Name("Font"),
		"Subtype":         pdf.Name("Type0"),
		"BaseFont":        pdf.Name(f.name),
		"Encoding":        pdf.Name("Identity-H"),
		"DescendantFonts": pdf.Array{cidRef},
		"ToUnicode":       toUnicodeRef,
	}); err != nil {
		return pdf.Ref{}, fmt.Errorf("writing Type0 font: %w", err)
	}

	return type0Ref, nil
}

// buildToUnicodeCMap generates a ToUnicode CMap that maps each GID back to
// its Unicode codepoint, enabling text selection and copy-paste in PDF viewers.
func buildToUnicodeCMap(fontName string, glyphMap map[rune]uint16) []byte {
	// Invert: GID → first rune that maps to it.
	gidToRune := make(map[uint16]rune, len(glyphMap))
	for r, gid := range glyphMap {
		if _, exists := gidToRune[gid]; !exists {
			gidToRune[gid] = r
		}
	}

	type pair struct {
		gid uint16
		r   rune
	}
	pairs := make([]pair, 0, len(gidToRune))
	for gid, r := range gidToRune {
		pairs = append(pairs, pair{gid, r})
	}

	var sb strings.Builder
	sb.WriteString("/CIDInit /ProcSet findresource begin\n12 dict begin\nbegincmap\n")
	sb.WriteString("/CIDSystemInfo << /Registry (Adobe) /Ordering (UCS) /Supplement 0 >> def\n")
	fmt.Fprintf(&sb, "/CMapName /%s-UCS def\n", fontName)
	sb.WriteString("/CMapType 2 def\n1 begincodespacerange\n<0000> <FFFF>\nendcodespacerange\n")

	const batchSize = 100
	for i := 0; i < len(pairs); i += batchSize {
		end := i + batchSize
		if end > len(pairs) {
			end = len(pairs)
		}
		chunk := pairs[i:end]
		fmt.Fprintf(&sb, "%d beginbfchar\n", len(chunk))
		for _, p := range chunk {
			fmt.Fprintf(&sb, "<%04X> <%04X>\n", p.gid, uint32(p.r))
		}
		sb.WriteString("endbfchar\n")
	}

	sb.WriteString("endcmap\nCMapName currentdict /CMap defineresource pop\nend\nend\n")
	return []byte(sb.String())
}
