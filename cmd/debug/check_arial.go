//go:build ignore

package main

import (
    "fmt"
    "github.com/pdf-translator/pdf-translator/internal/renderer"
)

func main() {
    f, err := renderer.LoadTTFFont("/Library/Fonts/Arial Unicode.ttf")
    if err != nil {
        fmt.Println("Error:", err)
        return
    }
    
    testRunes := []rune{'A', 'Z', 'А', 'Я', 'а', 'я', 'Б', 'в', 'г', 'д'}
    for _, r := range testRunes {
        has := f.HasGlyph(r)
        fmt.Printf("U+%04X %c: glyph=%v\n", r, r, has)
    }
}
