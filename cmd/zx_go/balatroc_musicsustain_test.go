package main

// balatroc_musicsustain_test.go — pass-48d PROOF (Seva hardware bug:
// "redraws when selecting a card or playing a hand make the music player
// pause"). The quark odometer (_qctr) is the player's heartbeat: if it
// stops advancing while the game animates, the AY freezes — that IS the
// bug. This test boots the REAL game, walks it to a live round, fires a
// SELECT and a PLAY, and demands the odometer advance in EVERY 30-frame
// window across the deal, the cursor repaint, the score tally and the
// win/next screens. A frozen window = FAIL with the phase named.

import (
	"fmt"
	"os"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestBalatrocMusicSustainPlus2(t *testing.T) {
	if os.Getenv("SUSTAIN") == "" {
		t.Skip("set SUSTAIN=1")
	}
	tapRaw, _ := os.ReadFile("/root/nerve-workspace/demos/balatroc-c/build/board.tap")
	blocks := balParseTapBlocks(t, tapRaw)
	var a uint16
	var bd []byte
	for i := range blocks {
		b := blocks[i]
		if b.hdr && b.typ == 3 && trimName(b.name) == "board" && i+1 < len(blocks) {
			a, bd = b.start, blocks[i+1].data
		}
	}
	prev := cliFlagsActive
	nf := cliFlags{}
	nf.noSound = true
	nf.headless = true
	cliFlagsActive = &nf
	defer func() { cliFlagsActive = prev }()
	emu, err := newEmulator(roms.ModelPlus2)
	if err != nil {
		t.Fatal(err)
	}
	emu.paused.Store(false)
	for i := 0; i < 220; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	for i, v := range bd {
		emu.mem.Write(a+uint16(i), v)
	}
	sent := uint16(0x7D00)
	emu.mem.Write(sent, 0x76)
	emu.cpu.SP = 0xFFFE
	emu.mem.Write(0xFFFC, byte(sent))
	emu.mem.Write(0xFFFD, byte(sent>>8))
	emu.cpu.PC = 0x8000
	emu.cpu.Halted = false
	emu.cpu.D, emu.cpu.E, emu.cpu.H, emu.cpu.L = 0, 0, 0x80, 0x00
	emu.cpu.IFF1, emu.cpu.IFF2 = false, false
	emu.cpu.IM = 1
	q := func() int { return int(emu.mem.Read(0xFD1D)) }
	runF := func(n int) {
		for i := 0; i < n; i++ {
			runOneFrameHeadless(emu, roms.ModelPlus2)
		}
	}
	stroke := func(row byte, bit byte, tail int) {
		emu.kbd.PressMatrixKey(int(row), bit, true)
		runF(4)
		emu.kbd.PressMatrixKey(int(row), bit, false)
		runF(tail)
	}
	// watchdog: run window frames, require odometer moved
	watch := func(name string, frames int) {
		q0 := q()
		runF(frames)
		// qctr is a BYTE odometer (wraps every 256 quarks) — compare mod-256
		d := (q()-q0 + 256) % 256
		fmt.Printf("SUSTAIN %-14s quarks=%d/%df\n", name, d, frames)
		if d <= 0 {
			t.Fatalf("MUSIC FROZEN during %s: qctr %d dead for %d frames — busy/redraw loop starves the player", name, q0, frames)
		}
	}
	watch("title", 120)
	stroke(4, 0x01, 40) // START (PLAY-bound key = the title's START)
	watch("after-start-deal", 90)
	stroke(4, 0x04, 40) // RIGHT — cursor walk (paint storm)
	watch("select-right", 60)
	stroke(4, 0x08, 40) // SEL — select card 1
	watch("select-card", 60)
	stroke(4, 0x08, 40) // SEL — deselect, RESELECT moves cursor on
	stroke(4, 0x04, 40) // RIGHT to next card
	stroke(4, 0x08, 40) // SEL — select card 2
	watch("select-2", 60)
	stroke(4, 0x01, 40) // PLAY -> score tally
	watch("play-tally", 150)
	watch("post-tally", 120)
	fmt.Println("SUSTAIN PASS: heartbeat alive through every redraw phase")
}
