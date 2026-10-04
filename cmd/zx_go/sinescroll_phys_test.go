package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestSinescrollPhysBankProbe(t *testing.T) {
	syms := map[string]uint16{}
	bs, _ := os.ReadFile("/root/nerve-workspace/demos/sinescroll/sinescroll.sym")
	for _, ln := range strings.Split(string(bs), "\n") {
		name, rest, ok := strings.Cut(strings.TrimSpace(ln), ":")
		if !ok {
			continue
		}
		rest = strings.TrimSpace(rest)
		if strings.HasPrefix(rest, "EQU 0x") {
			if v, e := strconv.ParseUint(rest[6:], 16, 16); e == nil {
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

	// fingerprint: stamp byte at screen offset $0010 of the HIDDEN bank
	// via a poke through $C000 window after set-page, then check physical.
	states := map[byte]int{}
	seen := map[string]string{}
	loop := syms["loop"]
	for f := 0; f < 40; f++ {
		steps := 0
		for {
			steps++
			if emu.cpu.PC == loop && steps > 20 {
				break
			}
			if steps > 300000 {
				t.Fatal("runaway")
			}
			emu.cpu.StepInstructionWithIRQ()
		}
		bbk := emu.mem.Read(syms["bbk"])
		states[bbk]++
		dispBank := 5
		if bbk&0x08 != 0 {
			dispBank = 7
		}
		pageBank := int(bbk & 0x07)
		seen[fmt.Sprintf("%02X", bbk)] = fmt.Sprintf("disp=b%d page=$C000->b%d", dispBank, pageBank)
		if dispBank == pageBank {
			t.Fatalf("frame %d torn bbk=%02X", f, bbk)
		}
		// display bank content: should be intact render (some lit pixels)
		p := emu.mem.RAM8KPage(dispBank*2)[:8192]
		lit := 0
		for _, v := range p[:0x1800] {
			lit += bitCount(v)
		}
		if f%8 == 0 {
			fmt.Printf("f%d bbk=%02X disp=b%d page->b%d dispLit=%d\n", f, bbk, dispBank, pageBank, lit)
		}
	}
	fmt.Printf("states=%v\n%v\n", states, seen)
}

func bitCount(v byte) int {
	c := 0
	for v != 0 {
		c += int(v & 1)
		v >>= 1
	}
	return c
}
