package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

// TestSinescrollReal — ground-truth store dump: arm at gloop for cb==1,
// print every store until gloop again, straight from the shipped tap.
func TestSinescrollReal(t *testing.T) {
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
	for _, rng := range [][2]uint16{{0x8384,0x8396},{0x83AF,0x83D3},{0x83D4,0x83F8},{0x8379,0x8396}} {
		var sb strings.Builder
		for a := rng[0]; a < rng[1]; a++ {
			fmt.Fprintf(&sb, "%02X ", emu.mem.Read(a))
		}
		fmt.Printf("RAM@%04X: %s\n", rng[0], sb.String())
	}
	emu.cpu.SP = 0xFF00
	emu.cpu.PC = base
	stores := 0
	for f := 0; f < 120; f++ {
		for s := 0; s < 69888/4; s++ {
			pc := emu.cpu.PC
			op := emu.mem.Read(pc)
			isStore := (op >= 0x70 && op <= 0x77 && op != 0x76) || op == 0x36
			hl := emu.cpu.HL()
			emu.cpu.StepInstructionWithIRQ()
			if !isStore {
				continue
			}
			o := uint16(hl & 0x3FFF)
			y := int((o>>11)&3)*64 + int((o>>5)&7)*8 + int((o>>8)&7)
			cb := emu.mem.Read(syms["cb"])
			fmt.Printf("cb=%d pc=%04X op=%02X dst=%04X y=%d val=%02X B=%d C=%d IY=%04X DE=%04X\n",
				cb, pc, op, hl, y, emu.mem.Read(hl), emu.cpu.B, emu.cpu.C, emu.cpu.IY, emu.cpu.DE())
			stores++
			if stores >= 48 {
				f = 999
				break
			}
		}
		if f == 999 {
			break
		}
		// not stored yet: run a frame
		target := emu.cpu.Tstates() + 69888
		for emu.cpu.Tstates() < target && stores == 0 {
			emu.cpu.StepInstructionWithIRQ()
		}
	}
	fmt.Printf("real done stores=%d\n", stores)
}
