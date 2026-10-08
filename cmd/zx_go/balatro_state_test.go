package main

import (
	"fmt"
	"testing"
)

// statePoke writes a byte to a linear CPU address (globals live in the
// visible $8000-$BFFF window; there is no $Fxxx mirror — writing one hits
// nothing).
func statePoke(emu *emulator, addr uint16, v uint8) {
	emu.mem.Write(addr, v)
}

// TestBalatroCursorSelect2 drives CURSOR with step-accurate key presses and
// reads state through the linear window.
func TestBalatroCursorSelect2(t *testing.T) {
	syms := loadBalatroSyms(t)
	emu := bootBalatro(t, syms)
	balRunFrames(emu, 6)
	// start a round: title accepts SPACE (EV_PLAY) or ENTER (EV_SEL)
	balPress(t, emu, syms, "SPACE")
	balRunFrames(emu, 4)
	if m := balPeek(emu, syms["MODE"]); m != bMODE_PLAY {
		t.Fatalf("MODE=%d after SPACE, want PLAY", m)
	}
	pressOnce(t, emu, syms, "6")
		fmt.Printf("dbg6 MODE=%d REL=%d PC=%04X SELX=%d\n", balPeek(emu, syms["MODE"]), balPeek(emu, syms["REL"]), emu.cpu.PC, balPeek(emu, syms["SELX"]))
	if c := balPeek(emu, syms["CURSOR"]); c != 1 {
		t.Fatalf("CURSOR=%d after 6, want 1", c)
	}
	pressOnce(t, emu, syms, "4")
	if c := balPeek(emu, syms["CURSOR"]); c != 0 {
		t.Fatalf("CURSOR=%d after 4, want 0", c)
	}

}
