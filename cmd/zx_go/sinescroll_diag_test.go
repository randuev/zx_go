package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

// TestSinescrollDiag — hardware ladder rung 1: ZERO live paging. Paints the
// displayed bank (b5) DIRECT via $4xxx once, text stays until SPACE. If this
// reads fine on Seva's TV, all geometry+content is proven and the fault is
// 100% confined to live $7FFD paging. If it does NOT, the fault is in the
// paint path itself (window/bank layout), and we ladder deeper.
func TestSinescrollDiag(t *testing.T) {
	syms := map[string]uint16{}
	bs, err := os.ReadFile("/root/nerve-workspace/demos/sinescroll/sinescroll_diag.sym")
	if err != nil {
		t.Fatal(err)
	}
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
	raw, err := os.ReadFile("/root/nerve-workspace/demos/sinescroll/sinescroll_diag.tap")
	if err != nil {
		t.Fatal(err)
	}
	blocks := parseTap(raw)
	base := blocks[0][0].(uint16)
	code := blocks[0][1].([]byte)
	if len(code) < 13000 {
		for _, b := range blocks[1:] {
			if d, ok := b[1].([]byte); ok && len(d) > 13000 {
				base = b[0].(uint16)
				code = d
			}
		}
	}
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
	// run: paint happens once, then loop idles w/ key poll
	for f := 0; f < 400; f++ {
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
	}
	// bank5 direct bitmap: canonical $4000 decode
	p := emu.mem.RAM8KPage(10)
	lit := 0
	rows := [192]int{}
	for y := 0; y < 192; y++ {
		o := ((y & 7) << 8) + ((y & 0x38) << 2) + ((y & 0xC0) << 5)
		for xb := 0; xb < 32; xb++ {
			v := p[o+xb]
			for bt := 0; bt < 8; bt++ {
				if v&(0x80>>bt) != 0 {
					lit++
					rows[y]++
				}
			}
		}
	}
	bbk := emu.mem.Read(syms["bbk"])
	dg := emu.mem.Read(syms["diagf"])
	first, last := -1, -1
	for y := 0; y < 192; y++ {
		if rows[y] > 0 {
			if first < 0 {
				first = y
			}
			last = y
		}
	}
	fmt.Printf("DIAG: bbk=%02X diagf=%d lit=%d bandY=%d..%d rows83..111:%v\n", bbk, dg, lit, first, last, rows[83:112])
	// ASCII dump of band rows for human verification
	for y := 80; y < 115; y++ {
		o := ((y & 7) << 8) + ((y & 0x38) << 2) + ((y & 0xC0) << 5)
		line := ""
		for xb := 0; xb < 32; xb++ {
			v := p[o+xb]
			for bt := 0; bt < 8; bt++ {
				if v&(0x80>>bt) != 0 {
					line += "#"
				} else {
					line += "."
				}
			}
		}
		fmt.Printf("y%03d %s\n", y, line)
	}
	fmt.Printf("p=%d\n", int(emu.mem.Read(syms["p"]))|int(emu.mem.Read(syms["p"]+1))<<8)
	rgb := make([]byte, 256*192*3)
	for y := 0; y < 192; y++ {
		o := ((y & 7) << 8) + ((y & 0x38) << 2) + ((y & 0xC0) << 5)
		for xb := 0; xb < 32; xb++ {
			v := p[o+xb]
			for bt := 0; bt < 8; bt++ {
				if v&(0x80>>bt) != 0 {
					i := (y*256 + xb*8 + bt) * 3
					rgb[i], rgb[i+1], rgb[i+2] = 0xFF, 0xFF, 0xFF
				}
			}
		}
	}
	if err := writePNG("/tmp/sine_kiosk_bank5.png", 256, 192, rgb); err != nil {
		t.Fatalf("png: %v", err)
	}
	if bbk != 0x07 {
		t.Fatalf("bbk must stay $07 (no live paging), got %02X", bbk)
	}
	if dg != 1 {
		t.Fatal("paint-once flag not set")
	}
	// kiosk paints the h8 level ONCE at p=66 (~30 real glyphs of sparse
	// lowercase text): ~300 lit px is correct ink for this state. The old
	// 1200 bar was calibrated against the ROM-garbage build (zero-filled
	// CHARPTR8 -> sprite fetch read $0000), which read as dense noise.
	if lit < 250 || lit > 600 {
		t.Fatalf("ink %d outside expected h8-once range [250,600]", lit)
	}
	// ASCII dump of band rows for human verification
	for y := 80; y < 115; y++ {
		o := ((y & 7) << 8) + ((y & 0x38) << 2) + ((y & 0xC0) << 5)
		line := ""
		for xb := 0; xb < 32; xb++ {
			v := p[o+xb]
			for bt := 0; bt < 8; bt++ {
				line += "#"
				if v&(0x80>>bt) == 0 {
					line = line[:len(line)-1] + "."
				}
			}
		}
		fmt.Printf("y%03d %s\n", y, line)
	}
}
