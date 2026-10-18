package main

// balatroc_noisecount_test.go — SEVA 2026-10-18: "noise is still playing
// non stop in store as well". LAW: PT3 tempo = ONE pattern row (quark)
// per 50 Hz frame. This test stands the shop screen up (real tape, no
// keys) and counts quarks per frame via a pre-fetch hook on _q_doquark
// ($D49D, pass-46b map). PASS = avg quarks/frame within [0.8,1.1] and
// no frame with >2 quarks. Hardware runs the identical instruction stream
// at 3.5 MHz, so headless quarks/frame IS the hardware tempo.

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
	emu.cpu.PC = 0xA47E // shop_screen pass-46b
	emu.cpu.IFF1, emu.cpu.IFF2 = false, false
	emu.cpu.IM = 1
	q := 0
	// dump burn code bytes once loaded
	for pc := uint16(0xD49D); pc < 0xD4C5; pc += 8 {
		fmt.Printf("burn $%04X:", pc)
		for j := 0; j < 8; j++ {
			fmt.Printf(" %02X", emu.mem.Read(uint16(pc+uint16(j))))
		}
		fmt.Println()
	}
	emu.cpu.AddPreFetchHook("quark", func(pc uint16) {
		if pc == 0xD49D {
			q++
		}
	})
	defer emu.cpu.RemovePreFetchHook("quark")
	var perFrame []int
	for f := 0; f < 400; f++ {
		q = 0
		runOneFrameHeadless(emu, roms.ModelPlus2)
		perFrame = append(perFrame, q)
	}
	tot, max := 0, 0
	for _, v := range perFrame {
		tot += v
		if v > max {
			max = v
		}
	}
	avg := float64(tot) / float64(len(perFrame))
	fmt.Printf("shop quark tempo: total=%d avgqpf=%.3f max=%d\n", tot, avg, max)
	// histogram
	h := map[int]int{}
	for _, v := range perFrame {
		h[v]++
	}
	fmt.Println("hist:", h)
	if avg < 0.8 || avg > 1.1 || max > 2 {
		t.Fatalf("quark tempo off PT3 50 Hz law: avg=%.3f max=%d", avg, max)
	}
}
