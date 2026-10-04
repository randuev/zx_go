package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

// TestSinescrollWcatch — intercept every (hl)/(ix)/(iy) store during the
// paint window and flag writes whose address decodes to pixel rows > 115.
// Prints PC + dest + data for the first offenders.
func TestSinescrollWcatch(t *testing.T) {
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
	emu, err := newEmulator(roms.ModelPlus2)
	if err != nil {
		t.Fatal(err)
	}
	emu.paused.Store(false)
	for i := 0; i < 220; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	for i, v := range code {
		emu.mem.Write(base+uint16(i), v)
	}
	emu.cpu.SP = 0xFF00
	emu.cpu.PC = base
	decodeY := func(a uint16) (int, int) {
		o := uint16(a & 0x3FFF)
		third := (o >> 11) & 3
		prow := (o >> 8) & 7
		crow := (o >> 5) & 7
		x := o & 31
		return int(third)*64 + int(crow)*8 + int(prow), int(x) * 8
	}
	fmt.Printf("wcatch base=%04X gloop=%04X w8row=%04X w8step=%04X g_slow=%04X eraseband=%04X isr=%04X loop=%04X\n",
		base, syms["gloop"], syms["w8row"], syms["w8step"], syms["g_slow"], syms["eraseband"], syms["isr"], syms["loop"])
	hits := 0
	// run frame by frame; during frames, step and inspect stores
	for f := 0; f < 120 && hits < 12; f++ {
		pb := int(emu.mem.Read(syms["p"])) | int(emu.mem.Read(syms["p"]+1))<<8
		// paint happens after vsync; run whole frame stepping
		target := emu.cpu.Tstates() + 69888
		for emu.cpu.Tstates() < target {
			pc := emu.cpu.PC
			hl := emu.cpu.HL()
			op := emu.mem.Read(pc)
			emu.cpu.StepInstructionWithIRQ()
			// LD (HL),r = 0x70..0x77 (0x76=HALT is not a store)
			if op >= 0x70 && op <= 0x77 && op != 0x76 {
				if pc >= syms["loop"] && pc <= syms["isr"] && hl >= 0x4000 && hl < 0x5800 {
					y, _ := decodeY(hl)
					if y >= 116 && y <= 191 {
						got := emu.mem.Read(hl)
						hits++
						cb := emu.mem.Read(syms["cb"])
						ylat := emu.mem.Read(syms["ylat"])
						hh := emu.mem.Read(syms["hh"])
						gmask := emu.mem.Read(syms["gmask"])
						iy := emu.cpu.IY
						fmt.Printf("HIT f%d p%d pc=%04X dest=%04X y=%d cb=%d ylat=%d hh=%d gmask=%d IY=%04X DE=%04X val=%02X\n",
							f, pb, pc, hl, y, cb, ylat, hh, gmask, iy, emu.cpu.DE(), got)
						// what DEFTAB slot is IY into, and its bytes
						slot := (uint16(iy) - syms["DEFTAB"]) / 2
						fmt.Printf("  iy->DEFTAB[%d] bytes=%02X%02X want_row=%d canonical=%04X\n",
							slot, emu.mem.Read(uint16(iy)), emu.mem.Read(uint16(iy)+1),
							slot, 0x4000+((slot&7)<<8)+((slot&0x38)<<2)+((slot&0xC0)<<5))
						// guest DEFTAB words 92..104 and 148..160
						var s strings.Builder
						for r := 92; r <= 104; r += 4 {
							fmt.Fprintf(&s, " y%d:%02X%02X", r, emu.mem.Read(syms["DEFTAB"]+uint16(r*2)+1), emu.mem.Read(syms["DEFTAB"]+uint16(r*2)))
						}
						for r := 148; r <= 160; r += 4 {
							fmt.Fprintf(&s, " y%d:%02X%02X", r, emu.mem.Read(syms["DEFTAB"]+uint16(r*2)+1), emu.mem.Read(syms["DEFTAB"]+uint16(r*2)))
						}
						fmt.Println(" " + s.String())
						if hits >= 12 {
							break
						}
					}
				}
			}
		}
	}
	fmt.Printf("wcatch done hits=%d\n", hits)
}
