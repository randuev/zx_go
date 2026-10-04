package main

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestSinescrollInitDump(t *testing.T) {
	bs, _ := os.ReadFile("/root/nerve-workspace/demos/sinescroll/sinescroll.sym")
	syms := map[string]uint16{}
	for _, ln := range strings.Split(string(bs), "\n") {
		name, rest, ok := strings.Cut(strings.TrimSpace(ln), ":")
		if !ok {
			continue
		}
		if strings.HasPrefix(rest, "EQU 0x") {
			var v uint32
			fmt.Sscanf(rest[6:], "%x", &v)
			syms[name] = uint16(v)
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
	// snapshot rows 144-161 of both banks BEFORE starting the demo
	dump := func(tag string) {
		for _, pg := range []int{10, 14} {
			page := emu.mem.RAM8KPage(pg)
			lit := 0
			var rows []int
			for y := 144; y <= 161; y++ {
				o := ((y & 7) << 8) + ((y & 0x38) << 2) + ((y & 0xC0) << 5)
				for xb := 0; xb < 32; xb++ {
					if page[o+xb] != 0 {
						lit++
						rows = append(rows, y)
						break
					}
				}
			}
			fmt.Printf("%s page%d bandB-bytes=%d rows=%v\n", tag, pg, lit, rows[:min(8, len(rows))])
		}
		// DEFTAB words the h8 loop may use: print DEFTAB[56..127]
		fmt.Print("DEFTAB[56..127]: ")
		for y := 56; y <= 127; y += 8 {
			lo := emu.mem.Read(syms["DEFTAB"] + uint16(y*2))
			hi := emu.mem.Read(syms["DEFTAB"] + uint16(y*2) + 1)
			fmt.Printf("y%d:%02X%02X ", y, hi, lo)
		}
		fmt.Println()
	}
	dump("PRE-START")
	for i, v := range code {
		emu.mem.Write(base+uint16(i), v)
	}
	emu.cpu.SP = 0xFF00
	emu.mem.Write(0xFFFE, 0)
	emu.mem.Write(0xFFFF, 0)
	emu.cpu.PC = base
	runOneFrameHeadless(emu, roms.ModelPlus2)
	dump("FRAME1")
	// magic probe: write pattern through mem.Write at DEFTAB[ytop] addrs,
	// read back via mem.Read AND RAM8KPage(8) — separates emulator MMU
	// read-path artifacts from real runtime overwrite.
	for y := 80; y <= 88; y++ {
		emu.mem.Write(syms["DEFTAB"]+uint16(y*2), 0xAB)
		emu.mem.Write(syms["DEFTAB"]+uint16(y*2)+1, 0xCD)
	}
	fmt.Printf("MAGIC mem.Read @9980+80*2=%02X%02X; RAM8KPage(8)[0x1980+80*2]=%02X%02X\n",
		emu.mem.Read(syms["DEFTAB"]+160), emu.mem.Read(syms["DEFTAB"]+161),
		emu.mem.RAM8KPage(8)[0x1980+160], emu.mem.RAM8KPage(8)[0x1980+161])
	runOneFrameHeadless(emu, roms.ModelPlus2)
	dump("FRAME2")
	runOneFrameHeadless(emu, roms.ModelPlus2)
	dump("FRAME3")
	// raw DEFTAB bytes y80..103
	var s strings.Builder
	for y := 80; y <= 103; y++ {
		fmt.Fprintf(&s, " y%d:%02X%02X", y, emu.mem.Read(syms["DEFTAB"]+uint16(y*2)+1), emu.mem.Read(syms["DEFTAB"]+uint16(y*2)))
	}
	fmt.Println("RAW" + s.String())
}
