package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

// TestSinescrollGlyph — step through g_slow (entering-glyph path) for the
// glyph at cb=1 ylat=94 and print the entire instruction stream with
// register context. The B/IY seed mismatch is the target.
func TestSinescrollGlyph(t *testing.T) {
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
	gslow, gbump := syms["g_slow"], syms["gbump"]
	armed := false
	done := false
	for f := 0; f < 120 && !done; f++ {
		target := emu.cpu.Tstates() + 69888*4
		for emu.cpu.Tstates() < target && !done {
			pc := emu.cpu.PC
			if !armed && pc == gslow && emu.mem.Read(syms["cb"]) == 1 {
				armed = true
			}
			if armed {
				op := emu.mem.Read(pc)
				fmt.Printf("pc=%04X op=%02X HL=%04X A=%02X BC=%04X DE=%04X IY=%04X ylat=%d scrhix=%02X cb=%d hh=%d\n",
					pc, op, emu.cpu.HL(), emu.cpu.A, emu.cpu.BC(), emu.cpu.DE(), emu.cpu.IY,
					emu.mem.Read(syms["ylat"]), emu.mem.Read(syms["scrhix"]), emu.mem.Read(syms["cb"]), emu.mem.Read(syms["hh"]))
				if pc >= gbump {
					fmt.Println("--gbump--")
					done = true
				}
			}
			emu.cpu.StepInstructionWithIRQ()
		}
	}
	fmt.Println("END")
}
