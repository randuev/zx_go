package main

import (
	"fmt"
	"testing"
)

// Compare engine FONT6CI/FONT6IDX/FONT6S base vs the baked reference from
// unit symbols — if the engine's tables or sheet bytes disagree with
// font6bake, every glyph it prints is shifted.
func TestBalatroFontCmp(t *testing.T) {
	syms := loadBalatroSyms(t)
	emu := bootBalatro(t, syms)
	ci0 := int(balPeek(emu, syms["FONT6CI"])) + int(balPeek(emu, syms["FONT6CI"]+1))*256
	fmt.Printf("FONT6S=%04X FONT6CI=%04X ci0=%04X delta=%d\n",
		syms["FONT6S"], syms["FONT6CI"], ci0, ci0-int(syms["FONT6S"]))
	// byte 0..5 of ci0 sh0 row in engine RAM vs file
	// reference: ci3 (D) sheet: glyph 'D' rows; print engine bytes of ci0 row0:
	line := ""
	for i := 0; i < 14; i++ {
		line += fmt.Sprintf("%02X ", balPeek(emu, uint16(ci0)+uint16(i)))
	}
	fmt.Printf("engine ci0 sh0: %s\n", line)
	line = ""
	for i := 0; i < 14; i++ {
		line += fmt.Sprintf("%02X ", balPeek(emu, syms["FONT6S"]+uint16(i)))
	}
	fmt.Printf("engine FONT6S+0: %s\n", line)
	// FONT6IDX for 'A'.. 'Z'
	idx := ""
	for c := 'A'; c <= 'Z'; c++ {
		idx += fmt.Sprintf("%d ", balPeek(emu, syms["FONT6IDX"]+uint16(c-' ')))
	}
	fmt.Printf("IDX A-Z: %s\n", idx)
}
