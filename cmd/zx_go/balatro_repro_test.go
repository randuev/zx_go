package main

import (
	"fmt"
	"image/png"
	"os"
	"testing"
)

// Replicates Seva's screenshot state: title -> deal -> settle; checksum
// FONT6S to catch glyph-sheet corruption; capture board frame.
func TestBalatroRepro(t *testing.T) {
	syms := loadBalatroSyms(t)
	emu := bootBalatro(t, syms)
	pressOnce(t, emu, syms, "SPACE")
	for i := 0; i < 120 && balPeek(emu, syms["MODE"]) != bMODE_PLAY; i++ {
		balRunFrames(emu, 1)
	}
	symSheet := syms["FONT6S"]
	sheetLen := 54 * 112
	base := make([]byte, sheetLen)
	for i := 0; i < sheetLen; i++ {
		base[i] = balPeek(emu, symSheet+uint16(i))
	}
	for round := 0; round < 8; round++ {
		balRunFrames(emu, 60)
		for i := 0; i < sheetLen; i += 7 {
			if balPeek(emu, symSheet+uint16(i)) != base[i] {
				t.Fatalf("FONT6S CORRUPT at +%d ci%d PC=%04X", i, i/112, emu.cpu.PC)
			}
		}
	}
	fmt.Printf("repro MODE=%d PC=%04X sheetOK\n", balPeek(emu, syms["MODE"]), emu.cpu.PC)
	for i := 0; i < 8; i++ {
		fmt.Printf("HAND[%d]=%02X sel=%d\n", i, balPeek(emu, syms["HAND"]+uint16(i)), balPeek(emu, syms["SELMARK"]+uint16(i)))
	}
	f, err := os.Create("/root/nerve-workspace/demos/balatro/proofs/repro-board.png")
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, emu.renderFrame()); err != nil {
		t.Fatal(err)
	}
	f.Close()
}
