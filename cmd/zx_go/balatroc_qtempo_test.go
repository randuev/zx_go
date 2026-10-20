package main

// balatroc_qtempo_test.go — PASS 48f IM2 TEMPO GATE, rebased pass 48i.
// Seva 2026-10-19 (three reports): "music still slows down severely during
// redraw". The quark is ISR-driven: crt0 runs _music_boot, which installs
// the IM2 vsync ISR ticking _qctr once per frame. This gate boots LIVE
// (crt0, the only honest door since the tap became a single CODE block)
// and measures quark/frame ratio through the hottest repaint moments:
//   IDLE  : title poll loop spinning
//   DEAL  : shuffle+deal repaint (START stroke)
//   SCORE : score animation frames (select + PLAY)
// LAW: quark delta / frame delta >= 0.90 through EVERY phase. A poll
// player fails this at DEAL/SCORE; an ISR player cannot (hardware fact).
// Constants: board.map of tap 88914c78… (_qctr=$FA2A).

import (
	"fmt"
	"os"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestBalatrocQTempoPlus2(t *testing.T) {
	if os.Getenv("QTEMPO") == "" {
		t.Skip("set QTEMPO=1")
	}
	const qctr = 0xFA2A
	emu := balBootLive(t)
	runF := func(n int) {
		for i := 0; i < n; i++ {
			runOneFrameHeadless(emu, roms.ModelPlus2)
		}
	}
	q := func() int { return int(emu.mem.Read(qctr)) }
	phase := func(name string, n int) {
		q0 := q()
		runF(n)
		q1 := q()
		dq := (q1 - q0 + 256) % 256 // byte odometer
		r := float64(dq) / float64(n)
		fmt.Printf("%-6s frames=%d quarks=%d ratio=%.2f PC=$%04X\n",
			name, n, dq, r, emu.cpu.PC)
		if r < 0.90 {
			t.Fatalf("%s tempo STRETCHED: %d quarks / %d frames = %.2f",
				name, dq, n, r)
		}
	}
	runF(400) // title poll reached; music_boot installs the IM2 vector
	if im := emu.cpu.IM; im != 2 {
		t.Fatalf("title poll left IM=%d, want 2 (ISR music)", im)
	}
	fmt.Printf("IM2 installed: I=$%02X IM=%d\n", emu.cpu.I, emu.cpu.IM)
	phase("idle", 100)
	balStroke(emu, runF, 4, 0x01, 100) // START -> shuffle+deal repaint
	phase("post-deal", 100)
	balStroke(emu, runF, 4, 0x08, 6) // SEL
	balStroke(emu, runF, 4, 0x01, 60) // PLAY -> score animation
	phase("score", 100)
	fmt.Println("IM2 tempo exact through idle/deal/score: PASS")
}
