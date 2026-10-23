package main

// balatroc_statsfix_test.go — PASS 51 gates (Seva hardware review 2026-10-20):
//
// STATS=1   deck-statistics laws:
//   - suit rows carry REAL colors (hearts/diamonds ink RED on white),
//   - dealt/used cells are plain EMPTY white space — attr paper must be
//     WHITE everywhere on the grid rows (no inverted black holes),
//   - the deck grid sits at y120: pixel rows b13..b14 are an EMPTY band,
//   - hand titles print FULL: ink bytes exist beyond x63 in name rows
//     (abbrevs never reach past ~x48).
//
// DEALFL=1  deal-repaint idempotence: boot, let deal_anim finish with the
//   per-slot painter, then direct-call paint_hand (full repaint, the
//   pass-38 canonical) — hand-zone pixels AND attrs must be byte-identical
//   (paint_slot is visually exactly the full repaint, minus the flicker).
//   Negative control: ZX_TAP=<pass-50c tap> still passes this one (same
//   steady state by design) — the flicker itself is a hardware-only
//   symptom; this gate guards correctness of the incremental painter.

import (
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func balStatsFixSetup(t *testing.T) (*emulator, uint16, map[string]uint16) {
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
		if b.hdr && b.typ == 3 && trimName(b.name) == "core" && i+1 < len(blocks) {
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
balRingPreload(t, emu, tapRaw)
	sent := uint16(0x7D00)
	emu.mem.Write(sent, 0x76)
	emu.cpu.SP = 0xBFF5
	emu.mem.Write(0xBFF5, byte(sent))
	emu.mem.Write(0xBFF6, byte(sent>>8))
	emu.cpu.PC = boardAddr
	emu.cpu.IFF1, emu.cpu.IFF2 = false, false
	emu.cpu.IM = 1
	return emu, boardAddr, syms
}

func balRunF(t *testing.T, emu *emulator, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
}

func TestBalatrocStatsFixPlus2(t *testing.T) {
	if os.Getenv("STATS") == "" {
		t.Skip("set STATS=1")
	}
	emu, _, syms := balStatsFixSetup(t)
	stroke := func(row byte, bit byte, tail int) {
		emu.kbd.PressMatrixKey(int(row), bit, true)
		balRunF(t, emu, 4)
		emu.kbd.PressMatrixKey(int(row), bit, false)
		balRunF(t, emu, tail)
	}
	balRunF(t, emu, 150)
	// PASS 65 knock law (dbg-door proven): START enters the round only at
	// the poll edge; re-knock until flow leaves TITLE(5), then re-knock D
	// until the overlay latches. A single stroke lands in the splash-eat
	// window and the census then samples the BOARD — the old flakes.
	for k := 0; k < 40; k++ {
		stroke(4, 0x01, 80)
		if emu.mem.Read(syms["flow"]) == 0 {
			break
		}
	}
	for k := 0; k < 40; k++ {
		stroke(1, 0x04, 80)
		if emu.mem.Read(syms["g_stats"]) == 1 {
			break
		}
	}
	if emu.mem.Read(syms["g_stats"]) != 1 {
		t.Fatalf("stats did not open (g_stats=0 flow=%d)", emu.mem.Read(syms["flow"]))
	}
	pa := func(y, bx int) uint16 {
		return uint16(0x4000 + ((y & 7) << 8) + ((y & 0x38) << 2) + ((y & 0xC0) << 5) + bx)
	}
	// 1. grid pixels live at NEW position y120.., old y104..119 empty band
	oldBand, newGrid := 0, 0
	for y := 104; y < 120; y++ {
		for bx := 0; bx < 32; bx++ {
			if emu.mem.Read(pa(y, bx)) != 0 {
				oldBand++
			}
		}
	}
	for y := 120; y < 184; y++ {
		for bx := 0; bx < 32; bx++ {
			if emu.mem.Read(pa(y, bx)) != 0 {
				newGrid++
			}
		}
	}
	fmt.Printf("STATSFIX gapPx=%d gridPx=%d\n", oldBand, newGrid)
	savePNGAtomic("/tmp/statsfix_fail.png", emu.lastFrame())
	for y := 104; y < 120; y++ {
		for bx := 0; bx < 32; bx++ {
			if v := emu.mem.Read(pa(y, bx)); v != 0 {
				fmt.Printf("GAP y%d c%d=%02X ", y, bx, v)
			}
		}
	}
	fmt.Println()
	if oldBand != 0 {
		t.Errorf("gap band b13..b14 not empty: %d lit bytes", oldBand)
	}
	if newGrid == 0 {
		t.Error("deck grid did not paint at y120")
	}
	// 2. attr rows 15..22 all paper WHITE (no black holes anywhere)
	holes := 0
	for r := 15; r <= 22; r++ {
		for c := 0; c < 32; c++ {
			v := emu.mem.Read(0x5800 + uint16(r*32+c))
			if v&0x38 != 0x38 { // paper not white (flash bit ignored)
				holes++
			}
		}
	}
	fmt.Printf("STATSFIX nonWhitePaperCells=%d\n", holes)
	if holes != 0 {
		t.Errorf("black-hole cells on grid: %d cells w/o white paper", holes)
	}
	// 3. suit-row colors: rows s1 (y136→row17, D) and s2 (y152→row19, H)
	//    col0 attr must carry RED ink; s0 (row15 S) and s3 (row21 C) BLACK.
	redRows, blackRows := 0, 0
	for _, r := range []int{17, 19} {
		if emu.mem.Read(0x5800+uint16(r*32))&0x07 == 0x02 {
			redRows++
		}
	}
	for _, r := range []int{15, 21} {
		if emu.mem.Read(0x5800+uint16(r*32))&0x07 == 0x00 {
			blackRows++
		}
	}
	fmt.Printf("STATSFIX redRows=%d/2 blackRows=%d/2\n", redRows, blackRows)
	if redRows != 2 {
		t.Errorf("hearts/diamonds rows not RED ink: %d/2", redRows)
	}
	if blackRows != 2 {
		t.Errorf("spades/clubs rows not BLACK ink: %d/2", blackRows)
	}
	// 4. FULL titles: ink bytes beyond byte-col 8 (px>63) inside name rows
	far := 0
	for r := 4; r <= 12; r++ {
		for bx := 8; bx <= 11; bx++ { // px 64..95, before cols at 108
			for y := r * 8; y < r*8+8; y++ {
				if emu.mem.Read(pa(y, bx)) != 0 {
					far++
				}
			}
		}
	}
	fmt.Printf("STATSFIX nameFarPxBytes=%d\n", far)
	if far == 0 {
		t.Error("hand titles still abbreviated (no ink past px63 in name rows)")
	}
}

func TestBalatrocDealFlickPlus2(t *testing.T) {
	if os.Getenv("DEALFL") == "" {
		t.Skip("set DEALFL=1")
	}
	emu, _, syms := balStatsFixSetup(t)
	balRunF(t, emu, 150)
	emu.kbd.PressMatrixKey(4, 0x01, true)
	balRunF(t, emu, 4)
	emu.kbd.PressMatrixKey(4, 0x01, false)
	// PASS 65 knock law: START enters the round only at the poll edge.
	for k := 0; k < 40; k++ {
		emu.kbd.PressMatrixKey(4, 0x01, true)
		balRunF(t, emu, 4)
		emu.kbd.PressMatrixKey(4, 0x01, false)
		balRunF(t, emu, 60)
		if emu.mem.Read(syms["flow"]) == 0 {
			break
		}
	}
	balRunF(t, emu, 240) // full deal animation
	if emu.mem.Read(syms["deal_upto"]) != 8 {
		t.Fatalf("deal did not finish: deal_upto=%d PC=$%04X",
			emu.mem.Read(syms["deal_upto"]), emu.cpu.PC)
	}
	c0, c1 := 104>>3, (216+23)>>3
	capPx := func() []byte {
		out := make([]byte, 0, 48*18)
		for y := 120; y <= 167; y++ {
			for bx := c0; bx <= c1; bx++ {
				a := uint16(0x4000 + ((y & 7) << 8) + ((y & 0x38) << 2) + ((y & 0xC0) << 5) + bx)
				out = append(out, emu.mem.Read(a))
			}
		}
		for r := 15; r <= 20; r++ {
			for bx := c0; bx <= c1; bx++ {
				out = append(out, emu.mem.Read(0x5800+uint16(r*32+bx)))
			}
		}
		return out
	}
	a := capPx()
	// canonical full repaint — per-slot deal result must be identical
	sent := uint16(0x7D00)
	emu.cpu.Halted = false
	// no-arg direct call: sdcc balances push ix/pop ix internally —
	// ONLY the return address goes on the stack (same as CALL).
	emu.cpu.SP -= 2
	emu.mem.Write(emu.cpu.SP, byte(sent))
	emu.mem.Write(emu.cpu.SP+1, byte(sent>>8))
	balBankSel(t, emu, 0)
	emu.cpu.PC = syms["paint_hand"]
	g := 0
	for emu.cpu.PC != sent && emu.cpu.PC != sent+1 {
		emu.cpu.StepInstructionWithIRQ()
		if g++; g > 8_000_000 {
			t.Fatal("paint_hand runaway")
		}
	}
	b := capPx()
	diff := 0
	for i := range a {
		if a[i] != b[i] {
			diff++
		}
	}
	fmt.Printf("DEALFLICK zoneBytes=%d lit=%d residueAfterFullRepaint=%d\n",
		len(a), cntLit(a), diff)
	if cntLit(a) < 200 {
		t.Error("hand zone nearly empty — deal did not paint")
	}
	if diff != 0 {
		t.Errorf("per-slot deal differs from full repaint: %d bytes", diff)
	}
}
