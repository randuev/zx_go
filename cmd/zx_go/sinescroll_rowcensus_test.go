package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestSinescrollRowCensus(t *testing.T) {
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
	emu.mem.Write(0xFFFE, 0)
	emu.mem.Write(0xFFFF, 0)
	emu.cpu.PC = base
	loop := syms["loop"]
	for f := 0; f < 240; f++ {
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
		if f < 236 {
			continue
		}
		bbk := emu.mem.Read(syms["bbk"])
		bank := 5
		if bbk&0x08 != 0 {
			bank = 7
		}
		p := emu.mem.RAM8KPage(bank*2)[:8192]
		lvl := emu.mem.Read(syms["lvl"])
		pp := int(emu.mem.Read(syms["p"])) | int(emu.mem.Read(syms["p"]+1))<<8
		fmt.Printf("--- f%d lvl%d bbk=%02X p=%d dispBank=%d\n", f, lvl, bbk, pp, bank)
		for y := 80; y < 114; y++ {
			o := ((y & 7) << 8) + ((y & 0x38) << 2) + ((y & 0xC0) << 5)
			c := 0
			for xb := 0; xb < 32; xb++ {
				c += bits8(p[o+xb])
			}
			fmt.Printf("y%03d ink=%3d\n", y, c)
		}
	}
}
