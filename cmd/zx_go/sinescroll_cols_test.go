package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

// TestSinescrollCols — per-column cy census. For every byte column cb the
// paint loop uses cx=cb*8+half, cy=YOFF[cx], ytop=cy-half. Print them plus
// which display rows the bank actually has ink in. Pins the static lower
// band (y144+) to the exact table values that paint it.
func TestSinescrollCols(t *testing.T) {
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
	for f := 0; f < 100; f++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	p := int(emu.mem.Read(syms["p"])) | int(emu.mem.Read(syms["p"]+1))<<8
	lvl := emu.mem.Read(syms["lvl"])
	half := emu.mem.Read(syms["half"])
	fmt.Printf("COLS p=%d lvl%d half=%d\n", p, lvl, half)
	for cb := 0; cb < 32; cb++ {
		cx := cb*8 + int(half)
		cy := emu.mem.Read(syms["YOFF"] + uint16(cx))
		fmt.Printf(" cb%2d cx%3d cy%3d ytop=%3d\n", cb, cx, cy, int(cy)-int(half))
	}
	var s strings.Builder
	for cx := 0; cx < 264; cx += 8 {
		fmt.Fprintf(&s, " %d:%d", cx, emu.mem.Read(syms["YOFF"]+uint16(cx)))
	}
	fmt.Println("YOFF:" + s.String())
	for _, pg := range []int{10, 14} {
		page := emu.mem.RAM8KPage(pg)
		var rows []int
		for y := 0; y < 192; y++ {
			c := 0
			for xb := 0; xb < 32; xb++ {
				c += bits8(page[((y&7)<<8)+((y&0x38)<<2)+((y&0xC0)<<5)+xb])
			}
			if c > 0 {
				rows = append(rows, y)
			}
		}
		if len(rows) > 0 {
			fmt.Printf("bank pg%d inkrows=%d..%d sets=%d\n", pg, firstOf(rows), lastOf(rows), len(rows))
		} else {
			fmt.Printf("bank pg%d EMPTY\n", pg)
		}
	}
	fmt.Printf("bbk=%02X otmin=%d otmax=%d hh=%d\n", emu.mem.Read(syms["bbk"]),
		emu.mem.Read(syms["otmin"]), emu.mem.Read(syms["otmax"]), emu.mem.Read(syms["hh"]))
}
