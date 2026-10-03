package main

import (
	"crypto/md5"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

// TestSinescrollFrozenDisplay — v2 core proof. While eraseband+walk repaint
// the HIDDEN bank, the DISPLAYED bank must be byte-frozen: the eye sees only
// fully committed frames (Seva's v1g real-+2 photo showed fragmented text
// because v1 erased+repainted the DISPLAYED bank mid-frame).
// Chips on this core: bank5@$4000 = RAM8KPage(10), bank7@$C000 =
// RAM8KPage(14) (probe-proven: PA steers video, CPU windows never move).
func TestSinescrollFrozenDisplay(t *testing.T) {
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
	sym := func(n string) uint16 {
		v, ok := syms[n]
		if !ok {
			t.Fatalf("symbol %s missing — rebuild demos first", n)
		}
		return v
	}
	band, bsave := sym("eraseband"), sym("bandsave")

	raw, err := os.ReadFile("/root/nerve-workspace/demos/sinescroll/sinescroll.tap")
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

	visPage := func() int {
		if emu.mem.Read(sym("bbk"))&0x08 != 0 {
			return 14 // PA=1 -> bank7 displayed
		}
		return 10 // PA=0 -> bank5 displayed
	}
	pVal := func() int { return int(emu.mem.Read(sym("p"))) | int(emu.mem.Read(sym("p")+1))<<8 }
	lvl := func() byte { return emu.mem.Read(sym("lvl")) }
	hashPage := func(page int) [16]byte {
		return md5.Sum(emu.mem.RAM8KPage(page)[:6144])
	}

	paints, seen8, seen16 := 0, false, false
	violFreeze, violNoPaint := 0, 0
	stepBudget := 0
	for paints < 40 || !(seen8 && seen16) {
		if paints > 3000 {
			t.Fatalf("timeout: paints=%d seen8=%v seen16=%v", paints, seen8, seen16)
		}
		// park at this frame's paint start
		for {
			emu.cpu.StepInstructionWithIRQ()
			if emu.cpu.PC == band {
				break
			}
		}
		paints++
		if paints < 3 {
			continue // init still settling
		}
		pa := visPage()
		hidden := 24 - pa // 10 <-> 14
		hVis := hashPage(pa)
		hHid0 := hashPage(hidden)
		hiddenDirty := false
		cycles := 0
		for {
			emu.cpu.StepInstructionWithIRQ()
			cycles++
			if cycles > 90000 {
				stepBudget++
				if stepBudget > 3 {
					t.Fatalf("paint never completed")
				}
			}
			if hashPage(pa) != hVis {
				violFreeze++
				t.Errorf("DISPLAYED chip page%d changed MID-PAINT at +%d instr — freeze violated", pa, cycles)
				break
			}
			if hashPage(hidden) != hHid0 {
				hiddenDirty = true
			}
			if emu.cpu.PC == bsave || cycles > 200000 {
				break
			}
		}
		if !hiddenDirty {
			violNoPaint++
			t.Errorf("paint#%d left hidden chip page%d untouched — no draw", paints, hidden)
		}
		lv := lvl()
		if lv == 0 {
			seen8 = true
		} else {
			seen16 = true
		}
		if paints%10 == 0 || cycles > 60000 {
			fmt.Printf("FREEZE paint#%d PA=%d vis=%d cyc=%d p=%d lvl%d dirty=%v\n",
				paints, (emu.mem.Read(sym("bbk"))>>3)&1, pa, cycles, pVal(), lv, hiddenDirty)
		}
	}
	fmt.Printf("FROZEN: paints=%d violFreeze=%d violNoPaint=%d seen8=%v seen16=%v\n",
		paints, violFreeze, violNoPaint, seen8, seen16)
	if violFreeze > 0 || violNoPaint > 0 {
		t.Errorf("v2 frozen-display invariant violated")
	}
}
