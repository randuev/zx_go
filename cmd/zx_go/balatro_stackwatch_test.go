package main

import (
	"fmt"
	"testing"
)

// TestBalatroStackWatch drives an aggressive session and flags the first
// control transfer whose result PC leaves the code zone [$8000,SLTAB).
func TestBalatroStackWatch(t *testing.T) {
	syms := loadBalatroSyms(t)
	emu := bootBalatro(t, syms)
	balRunFrames(emu, 6)
	codeLo := syms["start"]
	codeHi := syms["SLTAB"]
	bad := func(pc, newpc uint16, op byte) {
		if newpc >= codeHi || newpc < codeLo {
			if pc >= codeLo && pc < codeHi {
				fmt.Printf("BADXFER pc=$%04X op=%02X -> $%04X SP=%04X\n",
					pc, op, newpc, emu.cpu.SP)
			}
		}
	}
	step := func() {
		for i := 0; i < 400000; i++ {
			pc := emu.cpu.PC
			op := balPeek(emu, pc)
			emu.cpu.StepInstructionWithIRQ()
			bad(pc, emu.cpu.PC, op)
		}
	}
	// start round
	pressOnce(t, emu, syms, "SPACE")
	step()
	// select 5 with ENTER only (engine: ENTER toggles current cursor card)
	for i := 0; i < 5; i++ {
		pressOnce(t, emu, syms, "ENTER")
		step()
	}
	fmt.Printf("prePlay MODE=%d NSEL=%d\n", balPeek(emu, syms["MODE"]), balPeek(emu, syms["NSEL"]))
	pressOnce(t, emu, syms, "SPACE") // play
	// after play, watch MODE writes with ring of PCs
	prevM := balPeek(emu, syms["MODE"])
	var ring [64]uint16
	rj := 0
	for i := 0; i < 2_000_000; i++ {
		pc := emu.cpu.PC
		ring[rj%64] = pc
		rj++
		m := balPeek(emu, syms["MODE"])
		if m != prevM {
			fmt.Printf("MODE %d->%d pc=$%04X ring:", prevM, m, pc)
			for j := 0; j < 64; j++ {
				if j%16 == 0 {
					fmt.Printf("\n  ")
				}
				fmt.Printf("%04X ", ring[(rj-64+j)%64])
			}
			fmt.Printf("\n")
			prevM = m
		}
		emu.cpu.StepInstructionWithIRQ()
	}
	// advance won -> next blind
	pressOnce(t, emu, syms, "ENTER")
	step()
	fmt.Printf("afterAdv MODE=%d BLINDIDX=%d\n", balPeek(emu, syms["MODE"]), balPeek(emu, syms["BLINDIDX"]))
	// quit
	pressOnce(t, emu, syms, "Q")
	step()
	fmt.Printf("afterQuit PC=%04X MODE=%d\n", emu.cpu.PC, balPeek(emu, syms["MODE"]))
}
