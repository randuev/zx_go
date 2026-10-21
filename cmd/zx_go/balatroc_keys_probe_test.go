package main

// balatroc_keys_probe_test.go — PASS 50 key-decode truth probe.
// Boots the real game, then presses each kmap cell one at a time and
// reads the observables (cursor, mute, g_stats, hand hole, selected,
// pixels) so we learn EXACTLY how PressMatrixKey maps row/bit on the
// +2 core and which EVs decode live. Throwaway diagnostic.

import (
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestBalatrocKeysProbe(t *testing.T) {
	if os.Getenv("KEYPROBE") == "" {
		t.Skip("set KEYPROBE=1")
	}
	tapRaw, _ := os.ReadFile("/root/nerve-workspace/demos/balatroc-c/build/board.tap")
	mapRaw, _ := os.ReadFile("/root/nerve-workspace/demos/balatroc-c/build/board.map")
	syms := map[string]uint16{}
	re := regexp.MustCompile(`_(\w+)\s+=\s+\$([0-9A-Fa-f]{4})`)
	for _, m := range re.FindAllStringSubmatch(string(mapRaw), -1) {
		var v uint64
		fmt.Sscanf(m[2], "%x", &v)
		syms[m[1]] = uint16(v)
	}
	blocks := balParseTapBlocks(t, tapRaw)
	var boardData []byte
	var boardAddr uint16
	for i := range blocks {
		b := blocks[i]
		if b.hdr && b.typ == 3 && trimName(b.name) == "core" && i+1 < len(blocks) {
			boardData, boardAddr = blocks[i+1].data, b.start
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
	runF := func(n int) {
		for i := 0; i < n; i++ {
			runOneFrameHeadless(emu, roms.ModelPlus2)
		}
	}
	for i := 0; i < 220; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	for i, v := range boardData {
		emu.mem.Write(boardAddr+uint16(i), v)
	}
balRingPreload(t, emu, tapRaw)
	stroke := func(row byte, bit byte, tail int) {
		emu.kbd.PressMatrixKey(int(row), bit, true)
		runF(4)
		emu.kbd.PressMatrixKey(int(row), bit, false)
		runF(tail)
	}
	rd := func(n string) int { return int(emu.mem.Read(syms[n])) }
	sent := uint16(0x7D00)
	emu.mem.Write(sent, 0x76)
	emu.cpu.SP = 0xBFF5
	emu.mem.Write(0xBFF5, byte(sent))
	emu.mem.Write(0xBFF6, byte(sent>>8))
	emu.cpu.PC = boardAddr
	emu.cpu.IFF1, emu.cpu.IFF2 = false, false
	emu.cpu.IM = 1
	runF(150)
	stroke(4, 0x01, 120) // START '0'
	fmt.Printf("after START: cursor=%d top=%d mute=%d gstats=%d sel0=%d\n",
		rd("cursor"), rd("top"), rd("mute"), rd("g_stats"), rd("selected"))
	type pr struct {
		name string
		row  byte
		bit  byte
	}
	for _, p := range []pr{
		{"LEFT-5 r4b4?", 4, 0x10},
		{"RIGHT-8 r4b8?", 4, 0x08},
		{"SEL-7 r4b2", 4, 0x04},
		{"SORT-S r1b2", 1, 0x02},
		{"STATS-D r1b4", 1, 0x04},
		{"MUTE-M r7b4", 7, 0x04},
		{"STATS-D2 r1b4", 1, 0x04},
		{"SEL-7b2 again", 4, 0x02},
	} {
		stroke(p.row, p.bit, 60)
		sel := 0
		for c := 0; c < 8; c++ {
			sel += int(emu.mem.Read(syms["selected"]+uint16(c))) & 1
		}
		fmt.Printf("%-16s cursor=%d mute=%d gstats=%d selbits=%d top=%d PC=$%04X\n",
			p.name, rd("cursor"), rd("mute"), rd("g_stats"), sel, rd("top"), emu.cpu.PC)
	}
}
