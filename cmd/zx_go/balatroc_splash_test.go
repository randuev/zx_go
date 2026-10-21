package main

// balatroc_splash_test.go — PASS 53 gates (SPLASH=1):
// The boss splash card (Seva 3+1: splash + red border, press-any-key
// with 5 s odometer timeout).
//   LEG 1: press → splash paints (big-name band, rule band, press-band
//          all carry ink), border latches DARK RED, dismiss key returns.
//   LEG 2: NO key → the qctr odometer must dismiss it: a bounded
//          direct-call span of >=240 odometer ticks proves the 5 s
//          auto-drop works on the chip clock, never hangs. Border must
//          stay RED after dismissal (ambient cue, pass-53 option 1).
// boss_id/blind untouched: reuse the pass-52 BOSS law via next_round.

import (
	"fmt"
	"os"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestBalatrocSplashPlus2(t *testing.T) {
	if os.Getenv("SPLASH") == "" {
		t.Skip("set SPLASH=1")
	}
	emu, syms := balBossSetup(t)
	run := func(frames int) {
		for i := 0; i < frames; i++ {
			runOneFrameHeadless(emu, roms.ModelPlus2)
		}
	}
	run(150)
	if _, ok := syms["boss_splash"]; !ok {
		t.Fatal("boss_splash symbol missing — splash not linked into this tap")
	}
	pa := func(y, bx int) uint16 {
		return uint16(0x4000 + ((y & 7) << 8) + ((y & 0x38) << 2) + ((y & 0xC0) << 5) + bx)
	}
	bandInk := func(y0, y1 int) int {
		n := 0
		for y := y0; y <= y1; y++ {
			for bx := 0; bx < 32; bx++ {
				if emu.mem.Read(pa(y, bx)) != 0 {
					n++
				}
			}
		}
		return n
	}

	// --- LEG 1: paint + key-dismiss -----------------------------------------
	emu.mem.Write(syms["ante"], 1)
	emu.mem.Write(syms["blind"], 1)
	balCallNoArg(t, emu, syms["next_round"]) // -> blind=2, boss picked
	if emu.mem.Read(syms["boss_id"]) == 0 {
		t.Fatal("no boss picked for splash")
	}
	// dismiss key held DOWN through the splash (it polls live matrix state)
	emu.kbd.PressMatrixKey(4, 0x01, true)
	sent := uint16(0x7D00)
	emu.cpu.Halted = false
	emu.cpu.SP -= 2
	emu.mem.Write(emu.cpu.SP, byte(sent))
	emu.mem.Write(emu.cpu.SP+1, byte(sent>>8))
	emu.cpu.PC = syms["boss_splash"]
	g := 0
	for emu.cpu.PC != sent && emu.cpu.PC != sent+1 {
		emu.cpu.StepInstructionWithIRQ()
		if g++; g > 2_000_000 {
			t.Fatalf("splash did not dismiss on key (guard %d, PC=$%04X)",
				g, emu.cpu.PC)
		}
	}
	// census the card BEFORE the board repaint overwrites it: but the
	// splash drew directly to screen and the sentinel HALT painted nothing
	// since — the screen still carries the card.
	big, rule, press := bandInk(56, 68), bandInk(88, 95), bandInk(152, 159)
	emu.kbd.PressMatrixKey(4, 0x01, false)
	fmt.Printf("SPLASH boss=%d bigInk=%d ruleInk=%d pressInk=%d border=%d\n",
		emu.mem.Read(syms["boss_id"]), big, rule, press,
		emu.ula.BorderColour&0x07)
	if big == 0 {
		t.Error("big boss-name band empty — text12 never ran")
	}
	if rule == 0 {
		t.Error("rule band empty — BOSSRULE never printed")
	}
	if press == 0 {
		t.Error("PRESS ANY KEY band empty")
	}
	if emu.ula.BorderColour&0x07 != 2 {
		t.Errorf("boss border not RED: %d", emu.ula.BorderColour&0x07)
	}

	// --- LEG 2: odometer timeout, NO key -----------------------------------
	// fresh boss round, nothing pressed: the loop must drop by itself.
	emu.mem.Write(syms["ante"], 2)
	emu.mem.Write(syms["blind"], 1)
	balCallNoArg(t, emu, syms["next_round"])
	q0 := uint32(emu.mem.Read(syms["qctr"])) | uint32(emu.mem.Read(syms["qctr"]+1))<<8
	emu.cpu.Halted = false
	emu.cpu.SP -= 2
	emu.mem.Write(emu.cpu.SP, byte(sent))
	emu.mem.Write(emu.cpu.SP+1, byte(sent>>8))
	emu.cpu.PC = syms["boss_splash"]
	g = 0
	for emu.cpu.PC != sent && emu.cpu.PC != sent+1 {
		emu.cpu.StepInstructionWithIRQ()
		if g++; g > 20_000_000 {
			t.Fatalf("splash HUNG past 20M steps — timeout broken (PC=$%04X)",
				emu.cpu.PC)
		}
	}
	q1 := uint32(emu.mem.Read(syms["qctr"])) | uint32(emu.mem.Read(syms["qctr"]+1))<<8
	tick := uint16(q1 - q0)
	fmt.Printf("SPLASHTIMEOUT ticks=%d (>=240 proves the 5 s drop)\n", tick)
	if tick < 240 {
		t.Errorf("auto-dismiss fired after only %d ticks (<240)", tick)
	}
	if tick > 300 {
		t.Errorf("auto-dismiss late: %d ticks (>300 = >6 s)", tick)
	}
	// ambient cue: the NEXT board repaint (paint_stage latch) must re-latch
	// RED while the boss blind is live. The sentinel HALT paints nothing, so
	// drive draw_board directly — that is the every-repaint law.
	balCallNoArg(t, emu, syms["draw_board"])
	fmt.Printf("SPLASH ambient border after board repaint: %d\n",
		emu.ula.BorderColour&0x07)
	if emu.ula.BorderColour&0x07 != 2 {
		t.Errorf("ambient border not RED during boss blind: %d",
			emu.ula.BorderColour&0x07)
	}
}
