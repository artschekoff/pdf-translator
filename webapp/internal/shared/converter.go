package shared

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

// ── Markdown → Blocks ────────────────────────────────────────────────────────

var (
	reHeading      = regexp.MustCompile(`^(#{1,6})\s+(.*)`)
	reHRule        = regexp.MustCompile(`^(\*{3,}|-{3,}|_{3,})\s*$`)
	reTableSep     = regexp.MustCompile(`^\|?\s*:?-{3,}:?\s*(\|\s*:?-{3,}:?\s*)*\|?\s*$`)
	reBlockquote   = regexp.MustCompile(`^>\s*(.*)`)
	reChecklist    = regexp.MustCompile(`^[-*+]\s+\[([ xX])\]\s+(.*)`)
	reUnordered    = regexp.MustCompile(`^[-*+]\s+(.*)`)
	reOrdered      = regexp.MustCompile(`^\d+\.\s+(.*)`)
	reCodeFence    = regexp.MustCompile("^```")
	reTableRow     = regexp.MustCompile(`\|`)
)

func newBlock(t string, data map[string]any) BlockData {
	return BlockData{ID: uuid.NewString(), Type: t, Data: data}
}

func isTableSeparator(line string) bool {
	return reTableSep.MatchString(line)
}

func isTableRow(line string) bool {
	return reTableRow.MatchString(strings.TrimSpace(line))
}

func parseTableRow(line string, colCount int) []string {
	inner := strings.TrimSpace(line)
	inner = strings.TrimPrefix(inner, "|")
	inner = strings.TrimSuffix(inner, "|")
	parts := strings.Split(inner, "|")
	cells := make([]string, len(parts))
	for i, p := range parts {
		cells[i] = strings.TrimSpace(p)
	}
	for len(cells) < colCount {
		cells = append(cells, "")
	}
	return cells
}

// MarkdownToBlocks converts a Markdown string to a slice of BlockData.
// Handles: headings, fenced code blocks, ordered/unordered lists (merged),
// blockquotes, horizontal rules, tables, checklists, paragraphs.
func MarkdownToBlocks(md string) []BlockData {
	if md == "" {
		return nil
	}

	lines := strings.Split(md, "\n")
	var blocks []BlockData

	inCodeBlock := false
	codeLang := ""
	var codeLines []string

	flushCode := func() {
		if len(codeLines) > 0 {
			data := map[string]any{"code": strings.Join(codeLines, "\n")}
			if codeLang != "" {
				data["language"] = codeLang
			}
			blocks = append(blocks, newBlock("code", data))
		}
		codeLines = nil
		codeLang = ""
		inCodeBlock = false
	}

	pushListItem := func(style, text string) {
		if len(blocks) > 0 {
			last := &blocks[len(blocks)-1]
			if last.Type == "list" {
				if s, _ := last.Data["style"].(string); s == style {
					items, _ := last.Data["items"].([]string)
					last.Data["items"] = append(items, text)
					return
				}
			}
		}
		blocks = append(blocks, newBlock("list", map[string]any{
			"style": style,
			"items": []string{text},
		}))
	}

	pushChecklistItem := func(checked bool, text string) {
		item := map[string]any{"text": text, "checked": checked}
		if len(blocks) > 0 {
			last := &blocks[len(blocks)-1]
			if last.Type == "checklist" {
				items, _ := last.Data["items"].([]map[string]any)
				last.Data["items"] = append(items, item)
				return
			}
		}
		blocks = append(blocks, newBlock("checklist", map[string]any{
			"items": []map[string]any{item},
		}))
	}

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		// fenced code block
		if reCodeFence.MatchString(line) {
			if inCodeBlock {
				flushCode()
			} else {
				inCodeBlock = true
				codeLang = strings.TrimSpace(line[3:])
			}
			continue
		}
		if inCodeBlock {
			codeLines = append(codeLines, line)
			continue
		}

		// heading
		if m := reHeading.FindStringSubmatch(line); m != nil {
			level := len(m[1])
			blocks = append(blocks, newBlock("header", map[string]any{
				"level": level,
				"text":  strings.TrimSpace(m[2]),
			}))
			continue
		}

		// horizontal rule
		if reHRule.MatchString(line) {
			blocks = append(blocks, newBlock("delimiter", map[string]any{}))
			continue
		}

		// markdown table (header row + separator on next line)
		if isTableRow(line) && i+1 < len(lines) && isTableSeparator(lines[i+1]) {
			headerCells := parseTableRow(line, 0)
			colCount := len(headerCells)
			i++ // skip separator
			content := [][]string{headerCells}
			for i+1 < len(lines) && isTableRow(lines[i+1]) && !isTableSeparator(lines[i+1]) {
				i++
				content = append(content, parseTableRow(lines[i], colCount))
			}
			// convert [][]string to []any for JSON compatibility
			anyContent := make([]any, len(content))
			for ri, row := range content {
				anyRow := make([]any, len(row))
				for ci, cell := range row {
					anyRow[ci] = cell
				}
				anyContent[ri] = anyRow
			}
			blocks = append(blocks, newBlock("table", map[string]any{
				"withHeadings": true,
				"content":      anyContent,
			}))
			continue
		}

		// blockquote
		if m := reBlockquote.FindStringSubmatch(line); m != nil {
			blocks = append(blocks, newBlock("quote", map[string]any{
				"text":    strings.TrimSpace(m[1]),
				"caption": "",
			}))
			continue
		}

		// checklist
		if m := reChecklist.FindStringSubmatch(line); m != nil {
			checked := strings.ToLower(m[1]) == "x"
			pushChecklistItem(checked, strings.TrimSpace(m[2]))
			continue
		}

		// unordered list
		if m := reUnordered.FindStringSubmatch(line); m != nil {
			pushListItem("unordered", strings.TrimSpace(m[1]))
			continue
		}

		// ordered list
		if m := reOrdered.FindStringSubmatch(line); m != nil {
			pushListItem("ordered", strings.TrimSpace(m[1]))
			continue
		}

		// blank line — skip
		if strings.TrimSpace(line) == "" {
			continue
		}

		// paragraph
		blocks = append(blocks, newBlock("paragraph", map[string]any{"text": line}))
	}

	if inCodeBlock {
		flushCode()
	}

	// assign sequential order
	for i := range blocks {
		blocks[i].Order = i
	}

	return blocks
}

// ── Blocks → Markdown ────────────────────────────────────────────────────────

func blockText(data map[string]any, key string) string {
	if v, ok := data[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// BlocksToMarkdown converts a slice of BlockData back to a Markdown string.
func BlocksToMarkdown(blocks []BlockData) string {
	if len(blocks) == 0 {
		return ""
	}

	var parts []string

	for _, block := range blocks {
		d := block.Data

		switch block.Type {
		case "header":
			level := 1
			if v, ok := d["level"]; ok {
				switch lv := v.(type) {
				case int:
					level = lv
				case float64:
					level = int(lv)
				}
			}
			if level < 1 {
				level = 1
			}
			if level > 6 {
				level = 6
			}
			parts = append(parts, strings.Repeat("#", level)+" "+blockText(d, "text"))

		case "paragraph":
			parts = append(parts, blockText(d, "text"))

		case "list":
			style, _ := d["style"].(string)
			switch items := d["items"].(type) {
			case []string:
				for idx, item := range items {
					if style == "ordered" {
						parts = append(parts, fmt.Sprintf("%d. %s", idx+1, item))
					} else {
						parts = append(parts, "- "+item)
					}
				}
			case []any:
				for idx, raw := range items {
					text := ""
					switch v := raw.(type) {
					case string:
						text = v
					case map[string]any:
						if s, ok := v["content"].(string); ok {
							text = s
						} else if s, ok := v["text"].(string); ok {
							text = s
						}
					}
					if style == "ordered" {
						parts = append(parts, fmt.Sprintf("%d. %s", idx+1, text))
					} else {
						parts = append(parts, "- "+text)
					}
				}
			}

		case "checklist":
			switch items := d["items"].(type) {
			case []map[string]any:
				for _, item := range items {
					checked := " "
					if c, ok := item["checked"].(bool); ok && c {
						checked = "x"
					}
					text := ""
					if s, ok := item["text"].(string); ok {
						text = s
					} else if s, ok := item["content"].(string); ok {
						text = s
					}
					parts = append(parts, fmt.Sprintf("- [%s] %s", checked, text))
				}
			case []any:
				for _, raw := range items {
					item, ok := raw.(map[string]any)
					if !ok {
						continue
					}
					checked := " "
					if c, ok := item["checked"].(bool); ok && c {
						checked = "x"
					}
					text := ""
					if s, ok := item["text"].(string); ok {
						text = s
					} else if s, ok := item["content"].(string); ok {
						text = s
					}
					parts = append(parts, fmt.Sprintf("- [%s] %s", checked, text))
				}
			}

		case "code":
			lang := blockText(d, "language")
			parts = append(parts, "```"+lang+"\n"+blockText(d, "code")+"\n```")

		case "quote":
			parts = append(parts, "> "+blockText(d, "text"))

		case "table":
			var rows [][]string
			switch content := d["content"].(type) {
			case [][]string:
				rows = content
			case []any:
				for _, rawRow := range content {
					var row []string
					switch r := rawRow.(type) {
					case []string:
						row = r
					case []any:
						for _, cell := range r {
							if s, ok := cell.(string); ok {
								row = append(row, s)
							} else {
								row = append(row, "")
							}
						}
					}
					rows = append(rows, row)
				}
			}
			if len(rows) == 0 {
				break
			}
			colCount := 0
			for _, r := range rows {
				if len(r) > colCount {
					colCount = len(r)
				}
			}
			fmtRow := func(cells []string) string {
				padded := make([]string, colCount)
				copy(padded, cells)
				for i := len(cells); i < colCount; i++ {
					padded[i] = ""
				}
				return "| " + strings.Join(padded, " | ") + " |"
			}
			withHeadings, _ := d["withHeadings"].(bool)
			var tableLines []string
			if withHeadings && len(rows) > 0 {
				tableLines = append(tableLines, fmtRow(rows[0]))
				sep := make([]string, colCount)
				for i := range sep {
					sep[i] = "---"
				}
				tableLines = append(tableLines, "| "+strings.Join(sep, " | ")+" |")
				for _, r := range rows[1:] {
					tableLines = append(tableLines, fmtRow(r))
				}
			} else {
				for _, r := range rows {
					tableLines = append(tableLines, fmtRow(r))
				}
			}
			parts = append(parts, strings.Join(tableLines, "\n"))

		case "delimiter":
			parts = append(parts, "---")

		default:
			if t := blockText(d, "text"); t != "" {
				parts = append(parts, t)
			}
		}
	}

	return strings.Join(parts, "\n\n")
}
