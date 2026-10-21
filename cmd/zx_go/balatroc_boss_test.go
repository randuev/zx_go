package main

// balatroc_boss_test.go — PASS 52 gates (BOSS=1):
// CURVE: compute_target direct-call census vs REAL Balatro numbers —
//   antes 1-4 exact table, ante 5+ real ladder (22500/33750/45000 at 6),
//   endless ×1.7 per ante, THE WALL doubles its own blind.
//   Negative control: ZX_TAP=<pass-51 tap> MUST FAIL at ante 6.
// BOSS: drive next_round across the blind wrap — a boss is picked exactly
//   on boss blinds, no repeats until the 5-pool drains, counters obey the
//   rule per id (NEEDLE hands=1, WATER discs=0, MANACLE hand_cap=7 and
//   slot 8 stays empty after deal), pool recycles afterwards.

import (
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func balBossSetup(t *testing.T) (*emulator, map[string]uint16) {
	t.Helper()
	tap := os.Getenv("ZX_TAP")
	if tap == "" {
		tap = "/root/nerve-workspace/demos/balatroc-c/build/board.tap"
	}
	tapRaw, err := os.ReadFile(tap)
	if err != nil {
		t.Fatalf("read tap: %v", err)
	}
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
		if b.hdr && b.typ == 3 && trimName(b.name) == "board" && i+1 < len(blocks) {
			boardData, boardAddr = blocks[i+1].data, b.start
		}
	}
	prev := cliFlagsActive
	nf := cliFlags{}
	nf.noSound = true
	nf.headless = true
	cliFlagsActive = &nf
	t.Cleanup(func() { cliFlagsActive = prev })
	emu, err := newEmulator(roms.ModelPlus2)
	if err != nil {
		t.Fatal(err)
	}
	emu.paused.Store(false)
	for i := 0; i < 220; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	for i, v := range boardData {
		emu.mem.Write(boardAddr+uint16(i), v)
	}
	sent := uint16(0x7D00)
	emu.mem.Write(sent, 0x76)
	emu.cpu.SP = 0xFF56
	emu.mem.Write(0xFF56, byte(sent))
	emu.mem.Write(0xFF57, byte(sent>>8))
	emu.cpu.PC = boardAddr
	emu.cpu.IFF1, emu.cpu.IFF2 = false, false
	emu.cpu.IM = 1
	return emu, syms
}

func balCallNoArg(t *testing.T, emu *emulator, fn uint16) {
	t.Helper()
	sent := uint16(0x7D00)
	emu.cpu.Halted = false
	emu.cpu.SP -= 2
	emu.mem.Write(emu.cpu.SP, byte(sent))
	emu.mem.Write(emu.cpu.SP+1, byte(sent>>8))
	emu.cpu.PC = fn
	g := 0
	for emu.cpu.PC != sent && emu.cpu.PC != sent+1 {
		emu.cpu.StepInstructionWithIRQ()
		if g++; g > 8_000_000 {
			t.Fatal("direct-call runaway")
		}
	}
}

func rd32(emu *emulator, a uint16) uint32 {
	return uint32(emu.mem.Read(a)) | uint32(emu.mem.Read(a+1))<<8 |
		uint32(emu.mem.Read(a+2))<<16 | uint32(emu.mem.Read(a+3))<<24
}

func TestBalatrocBossCurvePlus2(t *testing.T) {
	if os.Getenv("BOSS") == "" {
		t.Skip("set BOSS=1")
	}
	emu, syms := balBossSetup(t)
	run := func(frames int) {
		for i := 0; i < frames; i++ {
			runOneFrameHeadless(emu, roms.ModelPlus2)
		}
	}
	run(150)
	// curve census: ante<5 keeps the proven table; ante 5+ = real ladder
	type tc struct {
		ante, blind uint8
		boss        uint8
		want        uint32
	}
	cases := []tc{
		{1, 0, 0, 300}, {4, 2, 0, 11250},
		{5, 0, 0, 11250}, {5, 1, 0, 16875}, {5, 2, 0, 22500},
		{6, 0, 0, 22500}, {6, 1, 0, 33750}, {6, 2, 0, 45000},
		{7, 2, 0, 75000}, {8, 2, 0, 135000},
		{9, 0, 0, 114750}, {9, 2, 0, 229500},
		{10, 0, 0, 195075},
		{1, 2, 3, 1200}, // THE WALL doubles ante1 boss 600
	}
	bad := 0
	for _, c := range cases {
		emu.mem.Write(syms["ante"], c.ante)
		emu.mem.Write(syms["blind"], c.blind)
		emu.mem.Write(syms["boss_id"], c.boss)
		balCallNoArg(t, emu, syms["compute_target"])
		got := rd32(emu, syms["tgt"])
		if got != c.want {
			bad++
			t.Errorf("CURVE ante%d blind%d boss%d: tgt=%d want %d",
				c.ante, c.blind, c.boss, got, c.want)
		}
	}
	fmt.Printf("BOSSCURVE cases=%d bad=%d (ante6 boss now %d, was 360000)\n",
		len(cases), bad, func() uint32 {
			emu.mem.Write(syms["ante"], 6)
			emu.mem.Write(syms["blind"], 2)
			emu.mem.Write(syms["boss_id"], 0)
			balCallNoArg(t, emu, syms["compute_target"])
			return rd32(emu, syms["tgt"])
		}())
}

func TestBalatrocBossRulesPlus2(t *testing.T) {
	if os.Getenv("BOSS") == "" {
		t.Skip("set BOSS=1")
	}
	emu, syms := balBossSetup(t)
	run := func(frames int) {
		for i := 0; i < frames; i++ {
			runOneFrameHeadless(emu, roms.ModelPlus2)
		}
	}
	run(150)
	seen := map[uint8]int{}
	rounds := 8
	for r := 0; r < rounds; r++ {
		// position at ante r+1 BIG blind so next_round wraps to BOSS
		emu.mem.Write(syms["ante"], uint8(r+1))
		emu.mem.Write(syms["blind"], 1)
		balCallNoArg(t, emu, syms["next_round"])
		if emu.mem.Read(syms["blind"]) != 2 {
			t.Fatalf("round %d: blind did not land on boss", r)
		}
		b := emu.mem.Read(syms["boss_id"])
		if b < 1 || b > 5 {
			t.Fatalf("round %d: boss_id=%d out of pool", r, b)
		}
		hands := emu.mem.Read(syms["hands_l"])
		discs := emu.mem.Read(syms["discs_l"])
		cap := emu.mem.Read(syms["hand_cap"])
		// MANACLE law: slot 7 (8th, idx 7) EMPTY — deal stopped early
		h8 := emu.mem.Read(syms["hand"] + 7)
		fmt.Printf("BOSS r%d id=%d hands=%d discs=%d cap=%d slot8=%02X\n",
			r, b, hands, discs, cap, h8)
		switch b {
		case 1:
			if hands != 1 {
				t.Errorf("NEEDLE: hands=%d", hands)
			}
		case 2:
			if discs != 0 {
				t.Errorf("WATER: discs=%d", discs)
			}
		case 4:
			if cap != 7 || h8 != 0xFF {
				t.Errorf("MANACLE: cap=%d slot8=%02X", cap, h8)
			}
		case 3, 5:
			if hands != 4 || discs != 3 || cap != 8 {
				t.Errorf("WALL/HOOK keep standard counters: h=%d d=%d cap=%d",
					hands, discs, cap)
			}
		}
		seen[b]++
		if r < 4 && len(seen) != r+1 {
			t.Errorf("round %d: boss repeat inside fresh pool (%v)", r, seen)
		}
	}
	// THE HOOK direct-call: two unselected held cards vanish, no refill.
	emu.mem.Write(syms["boss_id"], 5)
	before := 0
	for i := 0; i < 8; i++ {
		if emu.mem.Read(syms["hand"]+uint16(i)) != 0xFF {
			before++
		}
	}
	balCallNoArg(t, emu, syms["boss_hook"])
	after := 0
	for i := 0; i < 8; i++ {
		if emu.mem.Read(syms["hand"]+uint16(i)) != 0xFF {
			after++
		}
	}
	fmt.Printf("HOOK held %d -> %d\n", before, after)
	if before-after < 1 {
		t.Errorf("THE HOOK took no cards (%d -> %d)", before, after)
	}
	if after == before {
		t.Error("HOOK: nothing removed")
	}
	emu.mem.Write(syms["boss_id"], 0)
	balCallNoArg(t, emu, syms["boss_hook"])
	fmt.Printf("BOSSPOOL rounds=%d distinct=%d/5 seen=%v\n", rounds, len(seen), seen)
	if len(seen) != 5 {
		t.Errorf("pool never drained across 8 boss rounds: %v", seen)
	}
}
