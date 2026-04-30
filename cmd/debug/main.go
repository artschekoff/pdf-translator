//go:build ignore

package main

import (
    "fmt"
    "github.com/pdf-translator/pdf-translator/internal/renderer"
)

func main() {
    f, err := renderer.LoadTTFFont("/Users/riskyworks/Library/Fonts/JetBrainsMono-Regular.ttf")
    if err != nil {
        fmt.Println("Error:", err)
        return
    }
    
    testRunes := []rune{'A', 'Z', 'А', 'Я', 'а', 'я'}
    for _, r := range testRunes {
        has := f.HasGlyph(r)
        fmt.Printf("U+%04X %c: glyph=%v  hex=%s\n", r, r, has, f.EncodeTextHex(string(r)))
    }
    fmt.Println("\nFont name:", f.HasGlyph('А'))
}
