package main

// balatroc_noisecount_test.go — SEVA 2026-10-18: "noise is still playing
// non stop in store as well". LAW: PT3 tempo = ONE pattern row (quark)
// per 50 Hz frame. Pass-48g REBASE: quarks are no longer pumped by any
// caller — _isr (_tick=$0xd6a0) is entered by hardware vsync at exactly
// 50 Hz once IM2 is installed by _main before the title. This gate boots
// through crt0, STARTs into the LIVE game, and counts hook hits on _tick
// per frame: idle window, then the HOT held-RIGHT cursor repeat (the
// machine-gun scenario SEva heard). PASS = avg in [0.8,1.5], no frame >3.
// Hardware runs the identical instruction stream at 3.5 MHz, so headless
// quarks/frame IS the hardware tempo. Constants: board.map of tap
// c9827c58… (_tick=$0xd6a0 _qctr=$0xFA2A).

import (
	"fmt"
	"os"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestBalatrocShopQuarkTempoPlus2(t *testing.T) {
	if os.Getenv("QTEMPO") == "" {
		t.Skip("set QTEMPO=1")
	}
	tapRaw, _ := os.ReadFile("/root/nerve-workspace/demos/balatroc-c/build/board.tap")
	blocks := balParseTapBlocks(t, tapRaw)
	var a uint16
	var bd, md []byte
	for i := range blocks {
		b := blocks[i]
		if b.hdr && b.typ == 3 && trimName(b.name) == "core" && i+1 < len(blocks) {
			a, bd = b.start, blocks[i+1].data
		}
		if b.hdr && b.typ == 3 && trimName(b.name) == "music" && i+1 < len(blocks) {
			md = blocks[i+1].data
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
	for i, v := range md {
		emu.mem.Write(0x6000+uint16(i), v)
	}
	// PASS 56 RING: preload pages through the MMU window (door law)
	balRingPreload(t, emu, tapRaw)

	sent := uint16(0x7D00)
	emu.mem.Write(sent, 0x76)
	emu.cpu.SP = 0xBFF5
	emu.mem.Write(0xBFF5, byte(sent))
	emu.mem.Write(0xBFF6, byte(sent>>8))
	emu.cpu.PC = 0x8000 // crt0 — honest entry
	emu.cpu.IFF1, emu.cpu.IFF2 = false, false
	emu.cpu.IM = 1
	runF := func(n int) {
		for i := 0; i < n; i++ {
			runOneFrameHeadless(emu, roms.ModelPlus2)
		}
	}
	stroke := func(row int, bit byte, tail int) {
		emu.kbd.PressMatrixKey(row, bit, true)
		runF(2)
		emu.kbd.PressMatrixKey(row, bit, false)
		runF(tail)
	}
	q := 0
	emu.cpu.AddPreFetchHook("quark", func(pc uint16) {
		if pc == balSym("tick") { // _tick from live board.map
			q++
		}
	})
	defer emu.cpu.RemovePreFetchHook("quark")
	runF(160)               // crt0 -> music_boot -> title
	stroke(4, 0x01, 140)  // START into the live game
	var perFrame []int
	for f := 0; f < 400; f++ {
		q = 0
		runOneFrameHeadless(emu, roms.ModelPlus2)
		perFrame = append(perFrame, q)
	}
	// HOT PATH (SEVA pass-47d): hold RIGHT for 60 frames then release —
	// measure quark rate WHILE the cursor repeats. In the ISR world the
	// rate must be EXACTLY 1/frame no matter what the cursor paints.
	emu.kbd.PressMatrixKey(4, 0x04, true)
	hot := []int{}
	for f := 0; f < 60; f++ {
		q = 0
		runOneFrameHeadless(emu, roms.ModelPlus2)
		hot = append(hot, q)
	}
	emu.kbd.PressMatrixKey(4, 0x04, false)
	hmax, hsum := 0, 0
	for _, v := range hot {
		hsum += v
		if v > hmax {
			hmax = v
		}
	}
	fmt.Printf("HOT held-RIGHT quarks: total=%d avg=%.1f max=%d\n", hsum, float64(hsum)/60, hmax)
	tot, max := 0, 0
	for _, v := range perFrame {
		tot += v
		if v > max {
			max = v
		}
	}
	avg := float64(tot) / float64(len(perFrame))
	fmt.Printf("live-game quark tempo: total=%d avgqpf=%.3f max=%d\n", tot, avg, max)
	h := map[int]int{}
	for _, v := range perFrame {
		h[v]++
	}
	fmt.Println("hist:", h)
	if avg < 0.8 || avg > 1.5 || max > 3 {
		t.Fatalf("quark tempo off PT3 50 Hz law: avg=%.3f max=%d", avg, max)
	}
	if hmax > 3 || float64(hsum)/60 < 0.8 {
		t.Fatalf("HOT cursor repeat tempo broken: avg=%.2f max=%d", float64(hsum)/60, hmax)
	}
}
