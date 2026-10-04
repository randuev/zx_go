package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

// TestSinescrollCadence — the stutter gate (Seva 2026-10-04: "still stutters").
// Steps instruction-by-instruction, records every vsync ISR entry: the
// scroll counter p, level, and T-states elapsed since the previous vsync.
// Laws:
//   - p MUST advance by exactly spd(lvl) at EVERY vsync while the level is
//     stable (h8:1 px/f, h16:2 px/f). A vsync where p stands still = a
//     skipped paint = the stutter Seva sees.
//   - inter-vsync T MUST be < ~1.5 frame (69888 T): a doubled interval =
//     a missed vsync (paint overran the frame).
// Reports per-level violation counts + the worst offenders.
func TestSinescrollCadence(t *testing.T) {
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
		if strings.HasPrefix(rest, "EQU 0x") {
			if v, e := strconv.ParseUint(strings.Fields(rest[6:])[0], 16, 16); e == nil {
				syms[name] = uint16(v)
			}
		}
	}
	for _, n := range []string{"isr", "p", "lvl", "loop", "META"} {
		if _, ok := syms[n]; !ok {
			t.Fatalf("symbol %s missing", n)
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

	// baked SPEED table: META+8 (LSPD per level)
	sp8 := emu.mem.Read(sym("META") + 8)
	sp16 := emu.mem.Read(sym("META") + 9)
	fmt.Printf("CADENCE spd: h8=%d h16=%d\n", sp8, sp16)

	type vs struct {
		n    int
		p    int
		lvl  byte
		busy uint64
	}
	var vss []vs
	frame := 0
	for frame < 300 {
		tB := emu.cpu.Tstates()
		runOneFrameHeadless(emu, roms.ModelPlus2)
		p := int(emu.mem.Read(sym("p"))) | int(emu.mem.Read(sym("p")+1))<<8
		lvl := emu.mem.Read(sym("lvl"))
		vss = append(vss, vs{n: frame, p: p, lvl: lvl, busy: emu.cpu.Tstates() - tB})
		frame++
	}
	// skip boot frames until the first paint advances p (initial paint
	// jumps 0->64; frames before it hold p=0 legitimately — not stutter)
	for len(vss) > 0 && vss[0].p == 0 {
		vss = vss[1:]
	}

	lvl0, lvl1, trans := 0, 0, 0
	skip0, skip1, dbl0, dbl1 := 0, 0, 0, 0
	worstBusy := uint64(0)
	var violStr []string
	for i := 1; i < len(vss); i++ {
		a, b := vss[i-1], vss[i]
		d := b.p - a.p
		if b.lvl != a.lvl {
			trans++
			continue
		}
		spd := int(sp8)
		if b.lvl == 1 {
			spd = int(sp16)
		}
		if b.lvl == 0 {
			lvl0++
			if d == 0 {
				skip0++
				violStr = append(violStr, fmt.Sprintf("STUT f%d lvl0 p stuck=%d busy=%dT", b.n, b.p, b.busy))
			} else if d > spd {
				dbl0++
				violStr = append(violStr, fmt.Sprintf("DBL f%d lvl0 p %d->%d busy=%dT", b.n, a.p, b.p, b.busy))
			}
			if b.busy > worstBusy && false {
				worstBusy = b.busy
			}
		} else {
			lvl1++
			if d == 0 {
				skip1++
				violStr = append(violStr, fmt.Sprintf("STUT f%d lvl1 p stuck=%d busy=%dT", b.n, b.p, b.busy))
			} else if d > spd {
				dbl1++
				violStr = append(violStr, fmt.Sprintf("DBL f%d lvl1 p %d->%d busy=%dT", b.n, a.p, b.p, b.busy))
			}
		}
	}
	fmt.Printf("CADENCE: frames=%d lvl0=%d skip=%d dbl=%d | lvl1=%d skip=%d dbl=%d | trans=%d worstBusy=%dT (budget 69888)\n",
		len(vss), lvl0, skip0, dbl0, lvl1, skip1, dbl1, trans, worstBusy)
	for i, s := range violStr {
		if i >= 24 {
			fmt.Printf("  ... %d more\n", len(violStr)-24)
			break
		}
		fmt.Println("  " + s)
	}
	if skip0 > 0 || dbl0 > 0 {
		t.Errorf("h8 scroll not smooth: %d stuck vsyncs, %d double-steps of %d stable frames", skip0, dbl0, lvl0)
	}
	if skip1 > 0 || dbl1 > 0 {
		t.Errorf("h16 scroll not smooth: %d stuck, %d double of %d stable frames", skip1, dbl1, lvl1)
	}
	if worstBusy > 60000 {
		t.Errorf("worst busy-frame %dT — too close to 69888 budget, real +2 will hiccup", worstBusy)
	}
}

// scratch: per-column census of the displayed page at h8 parks.
func TestSinescrollColCensus(t *testing.T) {
	syms := map[string]uint16{}
	bs, _ := os.ReadFile("/root/nerve-workspace/demos/sinescroll/sinescroll.sym")
	for _, ln := range strings.Split(string(bs), "\n") {
		name, rest, ok := strings.Cut(strings.TrimSpace(ln), ":")
		if !ok {
			continue
		}
		rest = strings.TrimSpace(rest)
		if strings.HasPrefix(rest, "EQU 0x") {
			if v, e := strconv.ParseUint(strings.Fields(rest[6:])[0], 16, 16); e == nil {
				syms[name] = uint16(v)
			}
		}
	}
	raw, _ := os.ReadFile("/root/nerve-workspace/demos/sinescroll/sinescroll.tap")
	blocks := parseTap(raw)
	base, code := blocks[0][0].(uint16), blocks[0][1].([]byte)
	prev := cliFlagsActive
	nf := cliFlags{}
	nf.noSound = true
	cliFlagsActive = &nf
	defer func() { cliFlagsActive = prev }()
	emu, _ := newEmulator(roms.ModelPlus2)
	emu.paused.Store(false)
	for i := 0; i < 220; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	for i, v := range code {
		emu.mem.Write(base+uint16(i), v)
	}
	emu.cpu.SP = 0xFF00
	emu.cpu.PC = base
	lhalt := syms["lhalt"]
	for f := 0; f < 400; f++ {
		p0 := emu.cpu.PC
		_ = p0
		for {
			tS := emu.cpu.Tstates()
			budget := uint64(frameTStatesForModel(roms.ModelPlus2))
			for emu.cpu.Tstates()-tS < budget {
				if emu.cpu.PC == lhalt && f > 0 {
					goto parked
				}
				emu.cpu.StepInstructionWithIRQ()
			}
		}
	parked:
		if f < 380 || f > 395 {
			continue
		}
		p := int(emu.mem.Read(syms["p"])) | int(emu.mem.Read(syms["p"]+1))<<8
		lvl := emu.mem.Read(syms["lvl"])
		if lvl != 0 {
			continue
		}
		cpg := 10
		if emu.mem.Read(syms["bbk"])&0x08 != 0 {
			cpg = 14
		}
		page := emu.mem.RAM8KPage(cpg)
		fmt.Printf("COL p=%d: ", p)
		for col := 0; col < 32; col++ {
			ink := 0
			for yr := 84; yr <= 110; yr++ {
				o := ((yr & 7) << 8) + ((yr & 0x38) << 2) + ((yr & 0xC0) << 5) + col
				b := page[o]
				if b != 0 {
					ink += bits8[b]
				}
			}
			fmt.Printf("%d ", ink)
		}
		fmt.Println()
	}
}

var bits8 = [256]int{}

func init() {
	for i := 0; i < 256; i++ {
		b := 0
		for j := 0; j < 8; j++ {
			if i&(1<<j) != 0 {
				b++
			}
		}
		bits8[i] = b
	}
}
