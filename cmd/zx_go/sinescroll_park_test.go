package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
	"github.com/conorarmstrong/zx_go/pkg/z80"
)

// TestSinescrollParkCadence — the honest stutter gate.
// Samples at the loop's HALT park (PC just past lhalt), i.e. exactly once
// per demo loop iteration, not at arbitrary frame-window tails. Law:
// at every stable-level park, p MUST advance by spd(lvl). Records the
// instruction-executed count and taken/rejected INT counters per park so
// a stuck p is explained by its true cause (overrun/lost-INT), not theory.
func TestSinescrollParkCadence(t *testing.T) {
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
			t.Fatalf("symbol %s missing", n)
		}
		return v
	}
	lhalt := sym("lhalt")
	pA := sym("p")
	lvlA := sym("lvl")

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
	sp8 := int(emu.mem.Read(sym("META") + 8))
	sp16 := int(emu.mem.Read(sym("META") + 9))

	type pk struct {
		n       int
		p       int
		lvl     byte
		insns   uint64
		taken   uint64
		rej     uint64
	}
	var pks []pk
	insns := uint64(0)
	// step instruction-wise, catch every park (PC lands at lhalt+1 after the
	// HALT executes and the IM2 ISR has retired).
	for len(pks) < 300 {
		// zx_go advances PC past executed HALT: parked PC == lhalt+1.
		guard := uint64(70908 * 4)
		wd := 0
		for guard > 0 {
			if emu.cpu.PC == lhalt+1 {
				wd++
				if wd >= 2 { // seen it twice: truly parked, not a fly-by
					break
				}
			} else {
				wd = 0
			}
			emu.cpu.StepInstructionWithIRQ()
			insns++
			guard--
		}
		if guard == 0 {
			t.Fatalf("never parked at lhalt+1=%04X (PC=%04X) — bad build", lhalt+1, emu.cpu.PC)
		}
		p := int(emu.mem.Read(pA)) | int(emu.mem.Read(pA+1))<<8
		lvl := emu.mem.Read(lvlA)
		pks = append(pks, pk{n: len(pks), p: p, lvl: lvl, insns: insns,
			taken: z80.IntFireCount, rej: z80.IntRejectCount})
	}

	stuck0, stuck1, trans := 0, 0, 0
	var vStr []string
	for i := 1; i < len(pks); i++ {
		a, b := pks[i-1], pks[i]
		if b.lvl != a.lvl {
			trans++
			continue
		}
		spd := sp8
		c := &stuck0
		if b.lvl == 1 {
			spd = sp16
			c = &stuck1
		}
		if b.p-a.p != spd {
			*c++
			vStr = append(vStr, fmt.Sprintf("PARK%d p %d->%d (d=%d want %d) lvl%d insns=%d taken=%d rej=%d",
				b.n, a.p, b.p, b.p-a.p, spd, b.lvl, b.insns-a.insns, b.taken-a.taken, b.rej-a.rej))
		}
	}
	fmt.Printf("PARKCADENCE: parks=%d stuck_h8=%d stuck_h16=%d trans=%d rej_total=%d\n",
		len(pks), stuck0, stuck1, trans, z80.IntRejectCount)
	for i, s := range vStr {
		if i >= 12 {
			fmt.Printf("  ... %d more\n", len(vStr)-12)
			break
		}
		fmt.Println("  " + s)
	}
	if stuck0 > 0 || stuck1 > 0 {
		t.Errorf("scroll not smooth: %d h8 parks, %d h16 parks off-cadence", stuck0, stuck1)
	}
}
