package main

import (
	"fmt"
	"testing"
)

func TestBalatroGotTrace(t *testing.T) {
	syms := loadBalatroSyms(t)
	emu := bootBalatro(t, syms)
	balRunFrames(emu, 6)
	got, loop, rnd := 0, 0, 0
	emu.cpu.AddPreFetchHook("balgot", func(pc uint16) {
		switch pc {
		case syms["sfGot"]:
			got++
			if got <= 4 || got > 48 {
				fmt.Printf("GOT%d A=%02X BC=%04X DE=%04X HL=%04X\n", got, emu.cpu.A, emu.cpu.BC(), emu.cpu.DE(), emu.cpu.HL())
			}
		case syms["sfLoop"]:
			loop++
			if loop <= 3 || loop%51 == 0 {
				fmt.Printf("LOOP%d A=%02X BC=%04X DE=%04X\n", loop, emu.cpu.A, emu.cpu.BC(), emu.cpu.DE())
			}
		case syms["newRound"]:
			fmt.Printf("NEWROUND rnd16_so_far=%d got_so_far=%d DECK0=%02X DECK1=%02X SEED=%02X%02X\n",
				rnd, got, balPeek(emu, syms["DECK"]), balPeek(emu, syms["DECK"]+1),
				balPeek(emu, syms["SEED"]+1), balPeek(emu, syms["SEED"]))
		case syms["sfInitLoop"]:
			if got > 0 {
				fmt.Printf("INITAGAIN b=%02X\n", emu.cpu.A)
			}
		case syms["rnd16"]:
			rnd++
			if rnd <= 3 {
				fmt.Printf("RND%d BC=%04X DE=%04X HL=%04X SP=%04X\n", rnd, emu.cpu.BC(), emu.cpu.DE(), emu.cpu.HL(), emu.cpu.SP)
			}
		}
	})
	k := balatroKeys["ENTER"]
	emu.kbd.PressMatrixKey(int(k[0]), k[1], true)
	for f := 0; f < 8; f++ {
		balRunFrames(emu, 1)
	}
	emu.kbd.PressMatrixKey(int(k[0]), k[1], false)
	balRunFrames(emu, 2)
	fmt.Printf("sfGot=%d sfLoop=%d rnd16=%d\n", got, loop, rnd)
	fmt.Printf("IDXJ=%02X IDXK=%02X SWAPT=%02X\n", balPeek(emu, syms["IDXJ"]), balPeek(emu, syms["IDXK"]), balPeek(emu, syms["SWAPT"]))
}
