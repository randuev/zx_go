package main

// balatroc_playq_test.go — SEVA video 2026-10-18: regular ~75 ms tonal
// bursts on the PLAY screen while nothing moves. Count quarks per true
// frame (headless frame = 69888 T = exactly 20 ms at 3.5 MHz) after the
// REAL boot->title->START flow reaches the first play screen. PASS:
// <=2.2 quarks/frame (PT3 50 Hz law with tolerance).

import (
	"fmt"
	"os"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestBalatrocPlayQuarkPlus2(t *testing.T) {
	if os.Getenv("PLAYQ") == "" {
		t.Skip("set PLAYQ=1")
	}
	tapRaw, _ := os.ReadFile("/root/nerve-workspace/demos/balatroc-c/build/board.tap")
	blocks := balParseTapBlocks(t, tapRaw)
	var a uint16
	var bd, md []byte
	for i := range blocks {
		b := blocks[i]
		if b.hdr && b.typ == 3 && trimName(b.name) == "board" && i+1 < len(blocks) {
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
	sent := uint16(0x7D00)
	emu.mem.Write(sent, 0x76)
	emu.cpu.SP = 0xFF56
	emu.mem.Write(0xFF56, byte(sent))
	emu.mem.Write(0xFF57, byte(sent>>8))
	emu.cpu.PC = 0xAA03 // title_screen pass-47
	emu.cpu.Halted = false
	emu.cpu.IFF1, emu.cpu.IFF2 = false, false
	emu.cpu.IM = 1
	runF := func(n int) {
		for i := 0; i < n; i++ {
			runOneFrameHeadless(emu, roms.ModelPlus2)
		}
	}
	runF(120) // title paints + music_boot runs
	q := 0
	emu.cpu.AddPreFetchHook("q", func(pc uint16) {
		if pc == 0xD629 {
			q++
		}
	})
	defer emu.cpu.RemovePreFetchHook("q")
	// START = row 4 bit 0 (key '4'); title accepts any key (shots show it)
	emu.kbd.PressMatrixKey(4, 0x01, true)
	runF(4)
	emu.kbd.PressMatrixKey(4, 0x01, false)
	runF(60) // land on the first play screen
	per := []int{}
	for f := 0; f < 300; f++ {
		q = 0
		runOneFrameHeadless(emu, roms.ModelPlus2)
		per = append(per, q)
	}
	tot, max := 0, 0
	for _, v := range per {
		tot += v
		if v > max {
			max = v
		}
	}
	avg := float64(tot) / float64(len(per))
	fmt.Printf("play-screen quarks: total=%d avgqpf=%.3f max=%d PC=$%04X hands=%d\n",
		tot, avg, max, emu.cpu.PC, emu.mem.Read(balSym("hands_l")))
	h := map[int]int{}
	for _, v := range per {
		h[v]++
	}
	fmt.Println("hist:", h)
	if avg > 2.2 || max > 4 {
		t.Fatalf("PLAY-screen quark runaway: avg=%.2f max=%d (~%.0f bursts/s)", avg, max, avg*50)
	}
}
