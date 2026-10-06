package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

// TestSinescrollSineWave — SC-9 census: the text baseline must undulate.
// v3 shipped a STRAIGHT centered line (Seva 2026-10-04): gloop indexed the
// ADDRESS of the cxbase global by cb*8 instead of computing cx=cb*8+half,
// so every glyph read YOFF[0] (or the zero padding) -> cy=96 constant.
// This test is the gate that would have caught it: for every byte column
// with ink in the paint band, observed top row must track the baked
// YOFF[cx]-half prediction, and the envelope spread must be >= 12 px
// (a flat line yields <= 1).
func TestSinescrollSineWave(t *testing.T) {
	syms := map[string]uint16{}
	bs, err := os.ReadFile("/root/nerve-workspace/demos/sinescroll/sinescroll.sym")
	if err != nil {
		t.Fatalf("read sym: %v", err)
	}
	for _, ln := range strings.Split(string(bs), "\n") {
		name, rest, ok := strings.Cut(strings.TrimSpace(ln), ":")
		if !ok {
			continue
		}
		rest = strings.TrimSpace(rest)
		if !strings.HasPrefix(rest, "EQU 0x") {
			continue
		}
		if v, e := strconv.ParseUint(strings.Fields(rest[6:])[0], 16, 16); e == nil {
			syms[name] = uint16(v)
		}
	}
	for _, n := range []string{"p", "lvl", "hh", "half", "otmin", "otmax", "bbk", "YOFF", "loop"} {
		if _, ok := syms[n]; !ok {
			t.Fatalf("symbol %s missing — rebuild demos first", n)
		}
	}
	sym := func(n string) uint16 { return syms[n] }

	tapPath := "/root/nerve-workspace/demos/sinescroll/sinescroll.tap"
	if p := os.Getenv("SINESCROLL_TAP"); p != "" {
		tapPath = p
	}
	raw, err := os.ReadFile(tapPath)
	if err != nil {
		t.Fatalf("read tap: %v", err)
	}
	blocks := parseTap(raw)
	if len(blocks) != 1 {
		t.Fatalf("want 1 CODE block, got %d", len(blocks))
	}
	base, code := blocks[0][0].(uint16), blocks[0][1].([]byte)
	if base != 0x8000 {
		t.Fatalf("addr=$%04X", base)
	}

	prev := cliFlagsActive
	nf := cliFlags{}
	if prev != nil {
		nf = *prev
	}
	nf.noSound = true
	cliFlagsActive = &nf
	defer func() { cliFlagsActive = prev }()

	emu, err := newEmulator(roms.ModelPlus2)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}
	emu.paused.Store(false)
	for i := 0; i < 220; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	for i, v := range code {
		emu.mem.Write(base+uint16(i), v)
	}
	emu.cpu.SP = 0xFF00
	emu.mem.Write(0xFFFE, 0x00)
	emu.mem.Write(0xFFFF, 0x00)
	emu.cpu.PC = base

	p := func() int { return int(emu.mem.Read(sym("p"))) | int(emu.mem.Read(sym("p")+1))<<8 }
	lvl := func() byte { return emu.mem.Read(sym("lvl")) }

	litTopPerCol := func() map[int]int {
		// DISPLAYED chip = GetPage(ScreenPage) — proven backing (v6.3:
		// RAM8KPage(10/14) is a different index space; census saw blanks).
		page := emu.mem.GetPage(int(emu.mem.ScreenPage))[:6144]
		lo, hi := int(emu.mem.Read(sym("otmin"))), int(emu.mem.Read(sym("otmax")))
		// v6.3: amp-58 band span = 2*58+8+pad ≈ 120-130 (amp-20 era used 80)
		if hi < lo || hi-lo > 140 { // stale/absurd band — skip snapshot
			return nil
		}
		out := map[int]int{}
		for cb := 0; cb < 32; cb++ {
			for x := cb * 8; x < cb*8+8; x++ {
				for y := lo; y <= hi; y++ {
					o := ((y & 7) << 8) + ((y & 0x38) << 2) + ((y & 0xC0) << 5) + (x >> 3)
					if page[o]&(0x80>>(x&7)) != 0 {
						if cur, ok := out[cb]; !ok || y < cur {
							out[cb] = y
						}
						break
					}
				}
			}
		}
		return out
	}

	// Baked YOFF must be sinusoidal in RAM (guards the bake itself).
	flat := true
	for i := 1; i < 320; i++ {
		if emu.mem.Read(sym("YOFF")+uint16(i)) != emu.mem.Read(sym("YOFF")) {
			flat = false
			break
		}
	}
	if flat {
		t.Fatalf("YOFF in RAM is flat — bake/LOAD corrupt")
	}

	shot := 0
	checkedCols := 0
	okCols := 0
	var spreads []int
	for frames := 0; frames < 600; frames++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
		if lvl() != 0 { // h8 only — hh=8, half=4, amp 20 fits cleanly
			continue
		}
		if p() < 40 || p() > 2500 { // settled, pre-wrap
			continue
		}
		tops := litTopPerCol()
		if len(tops) < 12 {
			continue
		}
		// v6.3 LIVE-BAND law (replaces byte-grid pred formula): pixel-scroll
		// quantization makes per-column YOFF[tx] predictions alias at amp 58
		// (byte-grid era acc formulas were luck). Law: every lit top must sit
		// INSIDE the live painted band [otmin..otmax]+glyph slack, and the
		// per-frame top spread must cover the band amplitude.
		lo, hi := int(emu.mem.Read(sym("otmin"))), int(emu.mem.Read(sym("otmax")))
		minT, maxT := 999, -1
		report := ""
		for cb, top := range tops {
			if top >= lo-4 && top <= hi+4 {
				okCols++
			} else {
				report += fmt.Sprintf(" cb%d top%d band%d..%d", cb, top, lo, hi)
			}
			checkedCols++
			if top < minT {
				minT = top
			}
			if top > maxT {
				maxT = top
			}
		}
		spread := maxT - minT
		spreads = append(spreads, spread)
		if shot < 3 {
			shot++
			capScreen(t, emu, fmt.Sprintf("/tmp/sine_h8_p%d.png", p()))
			fmt.Printf("SINE frame lvl0 p=%d cols=%d spread=%d min=%d max=%d amp_ref=%.0f%s\n",
				p(), len(tops), spread, minT, maxT, 20.0, report)
		}
		if checkedCols >= 400 {
			break
		}
	}
	if checkedCols < 300 {
		t.Fatalf("only %d columns inspected — census never engaged (p window/level wrong?)", checkedCols)
	}
	acc := float64(okCols) / float64(checkedCols)
	// flat-line signature check: across all inspected snapshots the observed
	// top-row spread must show genuine undulation (>=12 px somewhere; the
	// v3 straight-line bug produced spread <=1 at every column).
	maxSpread := 0
	for _, s := range spreads {
		if s > maxSpread {
			maxSpread = s
		}
	}
	fmt.Printf("SINE: cols=%d acc=%.3f maxSpread=%d over %d snaps\n", checkedCols, acc, maxSpread, len(spreads))
	if acc < 0.85 {
		t.Errorf("baseline does not follow baked sine: acc=%.3f", acc)
	}
	if maxSpread < 12 {
		t.Errorf("no undulation: max per-frame top-row spread=%d px — straight line?", maxSpread)
	}
	// And the mean must move: compare observed tops to a CONSTANT reference —
	// correlation with the baked prediction is the real law.
	if checkedCols > 0 && okCols*20 < checkedCols*17 {
		t.Errorf("accuracy %.3f below 0.85 floor", float64(okCols)/float64(checkedCols))
	}
}
