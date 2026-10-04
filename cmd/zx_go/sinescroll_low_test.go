package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

// TestSinescrollLow — catch EVERY (hl) store into rows >=128 with the full
// anchor context (HL before/after, B,C,IY,A) so the exact failing branch is
// visible. Runs a few frames of h8 paint only.
func TestSinescrollLow(t *testing.T) {
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
	hits := 0
	decodeY := func(a uint16) int {
		o := uint16(a & 0x3FFF)
		return int((o>>11)&3)*64 + int((o>>5)&7)*8 + int((o>>8)&7)
	}
	gspanAddr, hhAddr := syms["gspan"], syms["hh"]
	for f := 0; f < 8 && hits < 10; f++ {
		gpPrev := emu.mem.Read(gspanAddr)
		target := emu.cpu.Tstates() + 69888
		for emu.cpu.Tstates() < target {
			pc := emu.cpu.PC
			op := emu.mem.Read(pc)
			hl := emu.cpu.HL()
			b, c := emu.cpu.B, emu.cpu.C
			iy := emu.cpu.IY
			emu.cpu.StepInstructionWithIRQ()
			gp := emu.mem.Read(gspanAddr)
			if gp != gpPrev {
				fmt.Printf("GSPAN-CHANGE pc=%04X %d->%d hl=%04X de=%04X cb=%d hh=%d gmask=%d\n",
					pc, gpPrev, gp, hl, emu.cpu.DE(), emu.mem.Read(syms["cb"]), emu.mem.Read(hhAddr), emu.mem.Read(syms["gmask"]))
				gpPrev = gp
				if pc != syms["gloop"] && pc < syms["loop"] {
					fmt.Println("  !!! write outside gloop")
				}
			}
			if op >= 0x70 && op <= 0x77 && op != 0x76 && pc >= syms["w8row"] && pc <= syms["gbump"] {
				if hl >= 0x4000 && hl < 0x5800 {
					y := decodeY(hl)
					if y >= 128 {
						hits++
						fmt.Printf("LOW f%d pc=%04X dest=%04X y=%d val=%02X B=%d C=%d IY=%04X cb=%d ylat=%d scrhix=%02X bbk=%02X\\n",
							f, pc, hl, y, emu.mem.Read(hl), b, c, iy,
							emu.mem.Read(syms["cb"]), emu.mem.Read(syms["ylat"]),
							emu.mem.Read(syms["scrhix"]), emu.mem.Read(syms["bbk"]))
						if hits >= 10 {
							break
						}
					}
				}
			}
		}
	}
	fmt.Printf("low done hits=%d\\n", hits)
}
