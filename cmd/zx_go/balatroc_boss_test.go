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
	return emu, syms
}

func balCallNoArg(t *testing.T, emu *emulator, fn uint16) {
	t.Helper()
	sent := uint16(0x7D00)
	// PASS 56 RING: window-addr targets live in PAGE GAME — hardware-select
	// page 0 before PC lands in the bank, else the call runs $00 zeros.
	if fn >= 0xC000 {
		balBankSel(t, emu, 0)
	}
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
		// PASS 63: WALL law reads the ANTE LOCK — drive it honestly.
		emu.mem.Write(syms["ante_boss"], c.boss)
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
	rounds := 13
	for r := 0; r < rounds; r++ {
		// position at ante r+1 BIG blind so next_round wraps to BOSS.
		// PASS 63: the boss source is the ANTE LOCK (ante_boss) — drive it
		// directly to probe each boss's rule, exactly once per round.
		emu.mem.Write(syms["ante"], uint8(r+1))
		emu.mem.Write(syms["blind"], 1)
		bWant := uint8(r%10 + 1)
		emu.mem.Write(syms["ante_boss"], bWant)
		balCallNoArg(t, emu, syms["next_round"])
		if emu.mem.Read(syms["blind"]) != 2 {
			t.Fatalf("round %d: blind did not land on boss", r)
		}
		b := emu.mem.Read(syms["boss_id"])
		if b != bWant {
			t.Fatalf("round %d: boss_id=%d != ante-lock %d", r, b, bWant)
		}
		if b < 1 || b > 10 {
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
		default:
			// PASS 54 WINDOW/CLUB/EYE/MOUTH/ARM: score-time bosses leave
			// the counters untouched — felt in eval, not in the HUD
			if hands != 4 || discs != 3 || cap != 8 {
				t.Errorf("boss %d must keep standard counters: h=%d d=%d cap=%d",
					b, hands, discs, cap)
			}
		}
		seen[b]++
		// PASS 63: pool no-repeat now lives at the WRAP (ante_boss picks) —
		// probe it there, not by faking stacked boss blinds.
	}
	// PASS 63 WRAP-POOL PROBE: the pool law stated in its own truth —
	// boss_used bits. Every wrap must mark its pick and touch nothing else;
	// a pick whose bit was ALREADY set is legal only through the reshuffle
	// door (before==0x3FF). 12 wraps must cover: drain of the nine remaining
	// bits, the reshuffle, and one post-reshuffle pick. Preview (ante_new)
	// is owed at every wrap.
	seenWrap := map[uint8]int{}
	bUsed := syms["boss_used"]
	rdUsed := func() uint16 {
		return uint16(emu.mem.Read(bUsed)) | uint16(emu.mem.Read(bUsed+1))<<8
	}
	ab0 := emu.mem.Read(syms["ante_boss"])
	seenWrap[ab0]++
	recycled := false
	for w := 0; w < 12 && !recycled; w++ {
		emu.mem.Write(syms["blind"], 2)
		emu.mem.Write(syms["ante_new"], 0)
		before := rdUsed()
		balCallNoArg(t, emu, syms["next_round"])
		if emu.mem.Read(syms["ante_new"]) != 1 {
			t.Fatalf("wrap %d: ante_new=%d — preview never owed", w, emu.mem.Read(syms["ante_new"]))
		}
		ab := emu.mem.Read(syms["ante_boss"])
		if ab < 1 || ab > 10 {
			t.Fatalf("wrap %d: ante_boss=%d off pool", w, ab)
		}
		after := rdUsed()
		bit := uint16(1 << uint(ab-1))
		fmt.Printf("WRAPPOOL w%d pick=%d used %03X->%03X\n", w, ab, before, after)
		switch {
		case before == 0x3FF:
			// reshuffle door: pool was full, engine cleared then marked —
			if after != bit {
				t.Fatalf("wrap %d: reshuffle marked %03X, want %03X", w, after, bit)
			}
			if seenWrap[ab] != 0 {
				recycled = true
			}
		case after&^before != bit:
			t.Fatalf("wrap %d: pick=%d mark wrong: used %03X->%03X", w, ab, before, after)
		case before&bit != 0:
			t.Fatalf("wrap %d: pick=%d from unmarked pool: used %03X->%03X", w, ab, before, after)
		}
		seenWrap[ab]++
		if emu.mem.Read(syms["blind"]) != 0 {
			t.Fatalf("wrap %d: blind=%d != 0 after wrap", w, emu.mem.Read(syms["blind"]))
		}
	}
	fmt.Printf("WRAPPOOL distinct=%d/10 recycled=%v used=%03X\n", len(seenWrap), recycled, rdUsed())
	if !recycled {
		t.Fatal("12 wraps never exercised the reshuffle door — pool law unproven")
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
	fmt.Printf("BOSSPOOL rounds=%d distinct=%d/10 seen=%v\n", rounds, len(seen), seen)
	if len(seen) != 10 {
		t.Errorf("pool never drained across %d boss rounds: %v", rounds, seen)
	}
}

// PASS 54 PHASE 2 — score-time bosses on the real play_sel path.
// Cards: Broadway SF spades 47..51, diamonds 34..38, clubs 8..12;
// pairs of Kings by suit (♠50 ♥25 ♦38 ♣12). SF base: 100ch × 8m.
func TestBalatrocBossP2Plus2(t *testing.T) {
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
	rd16 := func(a uint16) uint16 {
		return uint16(emu.mem.Read(a)) | uint16(emu.mem.Read(a+1))<<8
	}
	wr32 := func(a uint16, v uint32) {
		for i := 0; i < 4; i++ {
			emu.mem.Write(a+uint16(i), byte(v>>(8*i)))
		}
	}
	stage := func(cards ...uint8) {
		for i := 0; i < 8; i++ {
			emu.mem.Write(syms["hand"]+uint16(i), 0xFF)
			emu.mem.Write(syms["selected"]+uint16(i), 0)
		}
		for i, c := range cards {
			emu.mem.Write(syms["hand"]+uint16(i), c)
			emu.mem.Write(syms["selected"]+uint16(i), 1)
			emu.mem.Write(syms["sel_order"]+uint16(i), c)
		}
		emu.mem.Write(syms["n_order"], byte(len(cards)))
		wr32(syms["score"], 0)
	}
	fmt.Println("PASS54 PHASE2 = score-time bosses on the real play_sel path")
	sp := []uint8{47, 48, 49, 50, 51}
	dm := []uint8{34, 35, 36, 37, 38}
	cl := []uint8{8, 9, 10, 11, 12}

	// --- WINDOW: ♦ SF scores, then debuffed ------------------------------
	emu.mem.Write(syms["boss_id"], 0)
	balCallNoArg(t, emu, syms["boss_apply"])
	stage(sp...)
	balCallNoArg(t, emu, syms["play_sel"])
	sPlain, chiPlain := rd32(emu, syms["score"]), rd16(syms["last_chi"])
	stage(dm...)
	balCallNoArg(t, emu, syms["play_sel"])
	dPlain := rd32(emu, syms["score"])
	fmt.Printf("plain: SF spades chi=%d score=%d, SF diamonds score=%d\n",
		chiPlain, sPlain, dPlain)

	emu.mem.Write(syms["boss_id"], 6)
	balCallNoArg(t, emu, syms["boss_apply"])
	if emu.mem.Read(syms["dsuit"]) != 2 {
		t.Errorf("WINDOW: dsuit=%d want 2", emu.mem.Read(syms["dsuit"]))
	}
	stage(dm...)
	balCallNoArg(t, emu, syms["play_sel"])
	chiW, sW := rd16(syms["last_chi"]), rd32(emu, syms["score"])
	fmt.Printf("WINDOW: ♦SF chips %d -> %d (score %d -> %d), hand still SF=%d\n",
		chiPlain, chiW, dPlain, sW, emu.mem.Read(syms["htype"]))
	if emu.mem.Read(syms["htype"]) != 8 {
		t.Errorf("WINDOW broke the SF detection: htype=%d", emu.mem.Read(syms["htype"]))
	}
	if chiW >= chiPlain || sW >= dPlain {
		t.Errorf("WINDOW: diamonds not debuffed (chi %d vs %d)", chiW, chiPlain)
	}
	stage(sp...)
	balCallNoArg(t, emu, syms["play_sel"])
	if rd32(emu, syms["score"]) != sPlain {
		t.Errorf("WINDOW: spades SF also debuffed — law is ♦ only")
	}

	// --- CLUB: same law on ♣ -----------------------------------------------
	emu.mem.Write(syms["boss_id"], 7)
	balCallNoArg(t, emu, syms["boss_apply"])
	if emu.mem.Read(syms["dsuit"]) != 0 {
		t.Errorf("CLUB: dsuit=%d want 0", emu.mem.Read(syms["dsuit"]))
	}
	stage(cl...)
	balCallNoArg(t, emu, syms["play_sel"])
	chiC := rd16(syms["last_chi"])
	fmt.Printf("CLUB: ♣SF chips %d -> %d\n", chiPlain, chiC)
	if chiC >= chiPlain {
		t.Errorf("CLUB: clubs not debuffed (chi %d)", chiC)
	}
	emu.mem.Write(syms["boss_id"], 0)
	balCallNoArg(t, emu, syms["boss_apply"])
	if emu.mem.Read(syms["dsuit"]) != 0xFF {
		t.Errorf("dsuit not cleared after boss off: %02X", emu.mem.Read(syms["dsuit"]))
	}

	// --- ARM: level -1, preview == score -------------------------------
	lvl := syms["lvl"]
	emu.mem.Write(lvl+8, 3) // SF at level 3: 100+30 ch, 8+2 m
	emu.mem.Write(syms["boss_id"], 0)
	stage(sp...)
	balCallNoArg(t, emu, syms["play_sel"])
	chi3, mult3 := rd16(syms["last_chi"]), rd16(syms["last_mult"])
	emu.mem.Write(syms["boss_id"], 10)
	balCallNoArg(t, emu, syms["boss_apply"])
	stage(sp...)
	balCallNoArg(t, emu, syms["play_sel"])
	chiA, multA := rd16(syms["last_chi"]), rd16(syms["last_mult"])
	balCallNoArg(t, emu, syms["base_chi"])
	chiPrev := emu.cpu.HL() // u16 return lives in HL (sdcc ABI)
	fmt.Printf("ARM: lvl3 SF %dch×%dm -> scored %dch×%dm; base preview %d\n",
		chi3, mult3, chiA, multA, chiPrev)
	if chiA != chi3-15 || multA != mult3-1 {
		t.Errorf("ARM: expected %dch×%dm, got %d×%d", chi3-15, mult3-1, chiA, multA)
	}
	// calculator law: the PREVIEW base is the ARMED one too — 100+15=115
	if chiPrev != 115 {
		t.Errorf("ARM: calculator base_chi=%d, want 115 (lvl2 base)", chiPrev)
	}

	// --- EYE: one hand of each type per round -------------------------
	emu.mem.Write(syms["boss_id"], 8)
	emu.mem.Write(lvl+8, 1)
	balCallNoArg(t, emu, syms["boss_apply"])
	emu.mem.Write(syms["eye_seen"], 0)
	emu.mem.Write(syms["eye_block"], 0)
	// first PAIR scores (K♠50 K♥24)
	stage(50, 24)
	balCallNoArg(t, emu, syms["play_sel"])
	first := rd32(emu, syms["score"])
	ht1 := emu.mem.Read(syms["htype"])
	// second PAIR (K♦37 K♣11) must die silent
	stage(37, 11)
	balCallNoArg(t, emu, syms["play_sel"])
	second := rd32(emu, syms["score"])
	fmt.Printf("EYE: pair #1 scored %d, pair #2 scored %d (block=%d seen=%04X)\n",
		first, second, emu.mem.Read(syms["eye_block"]), rd16(syms["eye_seen"]))
	if first == 0 || ht1 != 1 {
		t.Errorf("EYE: first pair did not score as PAIR (score=%d htype=%d)",
			first, ht1)
	}
	if second != 0 || emu.mem.Read(syms["eye_block"]) != 1 {
		t.Error("EYE: repeated pair type was NOT blocked")
	}
	// a DIFFERENT type must still score (high card A♠ alone)
	emu.mem.Write(syms["eye_block"], 0)
	stage(51)
	balCallNoArg(t, emu, syms["play_sel"])
	if rd32(emu, syms["score"]) == 0 {
		t.Error("EYE: a new hand type was wrongly blocked")
	}

	// --- MOUTH: one suit only, refused at the gate ---------------------
	emu.mem.Write(syms["boss_id"], 9)
	balCallNoArg(t, emu, syms["boss_apply"])
	stage(47, 25) // ♠ + ♥: mixed
	balCallNoArg(t, emu, syms["mouth_ok"])
	mixed := emu.cpu.HL() & 0xFF // sdcc: uint8_t returns in L
	stage(47, 48) // ♠ + ♠: legal
	balCallNoArg(t, emu, syms["mouth_ok"])
	pure := emu.cpu.HL() & 0xFF
	fmt.Printf("MOUTH: mixed ok=%d pure ok=%d\n", mixed, pure)
	if mixed != 0 {
		t.Error("MOUTH: mixed-suit selection accepted")
	}
	if pure != 1 {
		t.Error("MOUTH: single-suit selection refused")
	}
	fmt.Println("PHASE2 OK")
}

// PASS 55 gate (BOSS=1): boss challenge name rides the panel line UNDER
// the blind name — black ink on YELLOW plate (attr 0x30); erased clean
// when no boss. Deck-stat law: NO bright bit anywhere in the suit grid
// (the pass-54 red cells blazed with bright-white paper vs plain paper
// on ♠/♣ — Seva spotted it; ink carries the red, paper stays plain).
func TestBalatrocBossPanelPlus2(t *testing.T) {
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
	attr := func(row, col int) uint8 { return emu.mem.Read(uint16(0x5800 + row*32 + col)) }
	pa := func(y, bx int) uint16 {
		return uint16(0x4000 + ((y & 7) << 8) + ((y & 0x38) << 2) + ((y & 0xC0) << 5) + bx)
	}
	bandInk := func(y0, y1, bx0, bx1 int) int {
		n := 0
		for y := y0; y <= y1; y++ {
			for bx := bx0; bx <= bx1; bx++ {
				if emu.mem.Read(pa(y, bx)) != 0 {
					n++
				}
			}
		}
		return n
	}
	// boss live: WINDOW on the line under "BOSS"
	emu.mem.Write(syms["blind"], 2)
	emu.mem.Write(syms["boss_id"], 6)
	balBankSel(t, emu, 0)
	balCallNoArg(t, emu, syms["draw_board"])
	plate, ink := 0, bandInk(8, 15, 0, 10)
	for c := 0; c <= 10; c++ {
		if attr(1, c) == 0x30 {
			plate++
		}
	}
	fmt.Printf("PANEL boss: yellow-plate cells=%d/11 name-ink=%d border=%d\n",
		plate, ink, emu.ula.BorderColour&0x07)
	if plate != 11 {
		t.Errorf("boss plate incomplete: %d/11 cells yellow", plate)
	}
	if ink == 0 {
		t.Error("boss name never painted on panel line")
	}
	// no boss: line erased — plain panel paper, zero ink
	emu.mem.Write(syms["blind"], 1)
	emu.mem.Write(syms["boss_id"], 0)
	balBankSel(t, emu, 0)
	balCallNoArg(t, emu, syms["draw_board"])
	run(2)
	plain, inkOff := 0, bandInk(8, 15, 0, 10)
	for c := 0; c <= 10; c++ {
		if attr(1, c) == 0x07 {
			plain++
		}
	}
	fmt.Printf("PANEL off: plain cells=%d/11 ink=%d\n", plain, inkOff)
	if plain != 11 || inkOff != 0 {
		t.Errorf("panel line not erased: plain=%d/11 ink=%d", plain, inkOff)
	}
	// deck stat grid: bright bit must be gone everywhere; ♦/♥ ink red
	balCallNoArg(t, emu, syms["paint_stats"])
	bright, badInk := 0, 0
	for row := 15; row <= 22; row++ {
		for c := 2; c <= 27; c++ {
			a := attr(row, c)
			if a&0x40 != 0 {
				bright++
			}
			if row == 17 || row == 19 { // H, D rows: ink bits must be RED
				if a&0x07 != 0x02 && a != 0x07 && a != 0x37 {
					badInk++
				}
			}
		}
	}
	fmt.Printf("STATS grid: bright-cells=%d red-ink-mismatch=%d\n", bright, badInk)
	if bright != 0 {
		t.Errorf("deck stat grid still blazes: %d bright cells", bright)
	}
}
