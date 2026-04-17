package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/gen2brain/go-fitz"
	"github.com/pdf-translator/pdf-translator/internal/domain"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: debug <pdf-path>\n")
		os.Exit(1)
	}

	doc, err := fitz.New(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "open: %v\n", err)
		os.Exit(1)
	}
	defer doc.Close()

	text, err := doc.Text(0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "text: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("=== MuPDF Text (page 1) ===")
	for i, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) != "" {
			fmt.Printf("  Line %d: %q\n", i, domain.TruncateString(line, 100))
		}
	}
}
