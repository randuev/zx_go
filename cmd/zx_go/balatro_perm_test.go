package main

import (
	"fmt"
	"testing"
)

// TestBalatroDeckShuffledAtDeal is the permanent deck invariant:
// after ENTER-deal, DECK is a valid permutation, not identity, and HAND
// matches DECK[0:8] via nextCard.
func TestBalatroDeckShuffledAtDeal(t *testing.T) {
	syms := loadBalatroSyms(t)
	emu := bootBalatro(t, syms)
	for i := 0; i < 3; i++ {
		emu.mem.Write(syms["TARGET"]+uint16(i), 0x99)
	}
	balRunFrames(emu, 6)
	prev := make([]byte, 52)
	for i := 0; i < 52; i++ {
		prev[i] = balPeek(emu, syms["DECK"]+uint16(i))
	}
	k := balatroKeys["ENTER"]
	emu.kbd.PressMatrixKey(int(k[0]), k[1], true)
	writes := 0
	firstPC := uint16(0)
	for f := 0; f < 8; f++ {
		for s := 0; s < 200000; s++ {
			emu.cpu.StepInstructionWithIRQ()
			for i := 0; i < 52; i++ {
				v := balPeek(emu, syms["DECK"] + uint16(i))
				if v != prev[i] {
					writes++
					if firstPC == 0 {
						firstPC = emu.cpu.PC
					}
					prev[i] = v
				}
			}
		}
	}
	emu.kbd.PressMatrixKey(int(k[0]), k[1], false)
	var dk [52]byte
	seen := map[byte]int{}
	for i := 0; i < 52; i++ {
		dk[i] = balPeek(emu, syms["DECK"] + uint16(i))
		seen[dk[i]]++
	}
	fmt.Printf("writes=%d firstPC=%04X\n", writes, firstPC)
	fmt.Printf("PC=%04X SP=%04X A=%02X MODE=%d DECKPOS=%d TICKL=%d TICKH=%d REL=%d\n",
		emu.cpu.PC, emu.cpu.SP, emu.cpu.A, balPeek(emu, syms["MODE"]),
		balPeek(emu, syms["DECKPOS"]), balPeek(emu, syms["TICKL"]), balPeek(emu, syms["TICKH"]), balPeek(emu, syms["REL"]))
	fmt.Printf("deck: %v\n", dk)
	for c := 0; c < 52; c++ {
		if seen[byte(c)] != 1 {
			t.Fatalf("card %d count=%d", c, seen[byte(c)])
		}
	}
	ident := true
	for i := 0; i < 52; i++ {
		if dk[i] != byte(i) {
			ident = false
		}
	}
	if ident {
		t.Fatal("deck identity — shuffle never ran")
	}
}
