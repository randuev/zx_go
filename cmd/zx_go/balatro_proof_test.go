package main

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// Writes proof PNGs for the full session: title, dealt board, selection,
// score animation, WON banner, then a discard + game-over path.
func TestBalatroProof(t *testing.T) {
	syms := loadBalatroSyms(t)
	emu := bootBalatro(t, syms)
	dir := "/root/nerve-workspace/demos/balatro/proofs"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	save := func(name string) {
		f, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		// renderFrame composes the live ULA framebuffer; lastFrame would be
		// a stale cache because runOneFrameHeadless never renders.
		if err := png.Encode(f, emu.renderFrame()); err != nil {
			f.Close()
			t.Fatal(err)
		}
		f.Close()
		fmt.Printf("proof: %s MODE=%d PC=%04X\n", name, balPeek(emu, syms["MODE"]), emu.cpu.PC)
	}
	balRunFrames(emu, 6)
	save("01-title.png")
	// start the round and WAIT for the deal to finish, then force four aces
	pressOnce(t, emu, syms, "SPACE")
	for i := 0; i < 60 && balPeek(emu, syms["MODE"]) != bMODE_PLAY; i++ {
		balRunFrames(emu, 1)
	}
	balRunFrames(emu, 60) // let deal animation complete
	for i := 0; i < 4; i++ {
		emu.mem.Write(syms["HAND"]+uint16(i), byte(i*16+12))
	}
	emu.mem.Write(syms["HAND"]+4, byte(2*16+2))
	for i := 0; i < 5; i++ {
		balPeek(emu, syms["CURSOR"])
	}
	save("02-hand.png")
	// select from the LEFT edge: cursor starts at slot0 after a fresh deal
	pressOnce(t, emu, syms, "ENTER")
	balRunFrames(emu, 2)
	for i := 1; i < 5; i++ {
		pressOnce(t, emu, syms, "6")
		balRunFrames(emu, 2)
		pressOnce(t, emu, syms, "ENTER")
		balRunFrames(emu, 2)
	}
	save("03-selected.png")
	k := balatroKeys["SPACE"]
	emu.kbd.PressMatrixKey(int(k[0]), k[1], true)
	for i := 0; i < 4000000 && balPeek(emu, syms["MODE"]) != bMODE_SCORE; i++ {
		emu.cpu.StepInstructionWithIRQ()
	}
	emu.kbd.PressMatrixKey(int(k[0]), k[1], false)
	// catch the animation mid-flight: first scoring card lit, total not final
	for i := 0; i < 200000 && balPeek(emu, syms["SCIDX"]) == 0 && balPeek(emu, syms["MODE"]) == bMODE_SCORE; i++ {
		emu.cpu.StepInstructionWithIRQ()
	}
	save("05-score-mid.png")
	for i := 0; i < 60 && balPeek(emu, syms["MODE"]) != bMODE_WON; i++ {
		balRunFrames(emu, 1)
	}
	hl := ""
	for i := 0; i < 5; i++ {
		hl += fmt.Sprintf("%02X ", balPeek(emu, syms["HAND"]+uint16(i)))
	}
	pl := ""
	for i := 0; i < 5; i++ {
		pl += fmt.Sprintf("%02X ", balPeek(emu, syms["PLAYED"]+uint16(i)))
		sm := balPeek(emu, syms["SELMARK"]+uint16(i))
		hl += fmt.Sprintf("[sel%d=%d]", i, sm)
	}
	fmt.Printf("PLAYED=%s TOT=%d TARGET=%d HAND=%s NSEL=%d HTIDX=%d\n", pl, balBCD3(emu, syms["TOT"]), balBCD3(emu, syms["TARGET"]), hl, balPeek(emu, syms["NSEL"]), balPeek(emu, syms["HTIDX"]))
	save("06-won.png")
	// advance to blind 2, then lose deliberately for OVER proof
	pressOnce(t, emu, syms, "ENTER")
	balRunFrames(emu, 6)
	save("07-blind2.png")
	emu.mem.Write(syms["BLINDTAB"]+0, 0x99)
	emu.mem.Write(syms["BLINDTAB"]+1, 0x99)
	emu.mem.Write(syms["BLINDTAB"]+2, 0x99)
	balRunFrames(emu, 2)
	// clear a hand, select all and play 4 hands to lose
	for round := 0; round < 4; round++ {
		for i := 0; i < 8; i++ {
			emu.mem.Write(syms["HAND"]+uint16(i), byte((i%4)*16+1+(i/4)))
		}
		for i := 0; i < 5; i++ {
			pressOnce(t, emu, syms, "ENTER")
			balRunFrames(emu, 2)
			pressOnce(t, emu, syms, "6")
			balRunFrames(emu, 2)
		}
		pressOnce(t, emu, syms, "SPACE")
		balRunFrames(emu, 40)
	}
	save("08-over.png")
	fmt.Printf("proof done MODE=%d TOT=%d\n", balPeek(emu, syms["MODE"]), balBCD3(emu, syms["TOT"]))
}
