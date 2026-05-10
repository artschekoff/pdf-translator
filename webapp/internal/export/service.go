package export

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/yourorg/pdf-translator-webapp/config"
	"github.com/yourorg/pdf-translator-webapp/internal/documents"
	"github.com/yourorg/pdf-translator-webapp/internal/shared"
	"gorm.io/gorm"
)

func Export(db *gorm.DB, doc *documents.Document, format, theme string) (string, error) {
	exports := config.C.ExportsDir
	os.MkdirAll(exports, 0755)
	stem := strings.ReplaceAll(doc.Title, " ", "_")

	var blocks []shared.BlockData
	for _, page := range doc.Pages {
		for _, b := range page.Blocks {
			var data map[string]any
			json.Unmarshal(b.Data, &data)
			blocks = append(blocks, shared.BlockData{ID: b.ID, Type: b.Type, Data: data, Order: b.Order})
		}
	}
	md := shared.BlocksToMarkdown(blocks)

	switch format {
	case "markdown":
		p := filepath.Join(exports, stem+".md")
		return p, os.WriteFile(p, []byte(md), 0644)

	case "txt":
		txt := regexp.MustCompile(`[#*` + "`" + `>|_~\[\]!]`).ReplaceAllString(md, "")
		p := filepath.Join(exports, stem+".txt")
		return p, os.WriteFile(p, []byte(txt), 0644)

	case "pdf":
		mdPath := filepath.Join(exports, stem+".md")
		if err := os.WriteFile(mdPath, []byte(md), 0644); err != nil {
			return "", err
		}
		flag := "--light"
		if theme == "dark" {
			flag = "--dark"
		}
		if err := exec.Command(config.C.MdtoPdfScript, flag, mdPath).Run(); err != nil {
			return "", fmt.Errorf("mdtopdf: %w", err)
		}
		return filepath.Join(exports, stem+".pdf"), nil

	case "docx":
		mdPath := filepath.Join(exports, stem+".md")
		outPath := filepath.Join(exports, stem+".docx")
		if err := os.WriteFile(mdPath, []byte(md), 0644); err != nil {
			return "", err
		}
		return outPath, exec.Command("pandoc", mdPath, "-o", outPath).Run()

	case "html":
		mdPath := filepath.Join(exports, stem+".md")
		outPath := filepath.Join(exports, stem+".html")
		if err := os.WriteFile(mdPath, []byte(md), 0644); err != nil {
			return "", err
		}
		return outPath, exec.Command("pandoc", mdPath, "-o", outPath, "--standalone").Run()
	}

	return "", fmt.Errorf("unknown format: %s", format)
}
