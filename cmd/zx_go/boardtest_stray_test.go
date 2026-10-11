package main

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

// TestBoardtestStraySpy : boardtest v5.3 forensics for Seva's real-hardware
// photo of the delivered boardtest.tap :
//   - symbols near the "=" signs (cmd bar b23 + b22 above)
//   - dots near the S of SCORE (b1 y8..14) and the R of ROUND (b13)
// Run boardtest once to ground truth, BLANK the suspect bytes, re-run the
// whole program, and attribute every first lit write to its PC.
func TestBoardtestStraySpy(t *testing.T) {
	bin, err := os.ReadFile("/root/nerve-workspace/demos/balatro/boardtest.bin")
	if err != nil {
		t.Fatal(err)
	}
	prev := cliFlagsActive
	nf := cliFlags{}
	nf.noSound = true
	cliFlagsActive = &nf
	t.Cleanup(func() { cliFlagsActive = prev })
	emu, err := newEmulator(roms.ModelPlus2)
	if err != nil {
		t.Fatal(err)
	}
	emu.paused.Store(false)
	for i := 0; i < 220; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	load := func() {
		for i, v := range bin {
			emu.mem.Write(0x8000+uint16(i), v)
		}
		emu.mem.Write(0xBF00, 0x76) // halt pad
		emu.cpu.SP = 0xFF00
		emu.mem.Write(0xFF00, 0x00)
		emu.mem.Write(0xFF01, 0xBF)
		emu.cpu.PC = 0x8000
	}
	load()
	run := func(limit int) {
		for i := 0; i < limit; i++ {
			if balPeek(emu, emu.cpu.PC) == 0x76 {
				return
			}
			emu.cpu.StepInstructionWithIRQ()
		}
	}
	run(3_000_000)
	pb := func(y int) uint16 {
		return 0x4000 + uint16((y&7)<<8+(y&0x38)<<2+(y&0xC0)<<5)
	}
	addrsOf := func(y0, y1, c0, c1 int) []uint16 {
		var out []uint16
		for y := y0; y <= y1; y++ {
			for c := c0; c <= c1; c++ {
				out = append(out, pb(y)+uint16(c))
			}
		}
		return out
	}
	zones := []struct {
		name  string
		addrs []uint16
	}{
		{"b22-above-cmdbar", addrsOf(176, 183, 0, 39)},
		{"b23-cmdbar-full", addrsOf(184, 190, 0, 39)},
		{"b1-scorefull", addrsOf(8, 15, 0, 39)},
		{"b4-rscorefull", addrsOf(32, 39, 0, 39)},
		{"b0-blindchip", addrsOf(0, 7, 0, 19)},
		{"b1-cmdhi", addrsOf(8, 15, 20, 39)},
		{"b23-cmdbar", addrsOf(184, 190, 0, 39)},
		{"b2-chip1200line", addrsOf(16, 23, 0, 2)},
		{"b5-chip0line", addrsOf(40, 47, 0, 2)},
		{"b1-SCOREline", addrsOf(8, 15, 0, 15)},
		{"b13-ROUNDline", addrsOf(104, 111, 5, 13)},
	}
	written := map[uint16]string{}
	for _, z := range zones {
		for _, a := range z.addrs {
			emu.mem.Write(a, 0)
		}
	}
	load() // re-run : cls+redraw everything
	for i := 0; i < 3_000_000; i++ {
		pc := emu.cpu.PC
		if balPeek(emu, pc) == 0x76 && pc != 0x8000 {
			break
		}
		emu.cpu.StepInstructionWithIRQ()
		for _, z := range zones {
			for _, a := range z.addrs {
				if written[a] == "" {
					if v := balPeek(emu, a); v != 0 {
						written[a] = fmt.Sprintf("pc=$%04X hl=$%04X ix=$%04X", emu.cpu.PC, emu.cpu.HL(), emu.cpu.IX)
					}
				}
			}
		}
	}
	for band := 0; band < 24; band++ {
		fmt.Printf("== b%d ==\n", band)
		y0 := band * 8
		for py := 0; py < 8; py++ {
			y := y0 + py
			line := ""
			for c := 0; c < 40; c++ {
				v := balPeek(emu, pb(y)+uint16(c))
				line += fmt.Sprintf("%08b", v)
			}
			fmt.Printf("y%03d %s\n", y, strings.ReplaceAll(strings.ReplaceAll(line, "0", "."), "1", "#"))
		}
	}
	fmt.Println("ATTRS b0-b5 :")
	for b := 0; b < 6; b++ {
		line := ""
		for c := 0; c < 40; c++ {
			line += fmt.Sprintf("%02X ", balPeek(emu, 0x5800+uint16(b*32+c)))
		}
		fmt.Printf("a%d %s\n", b, line)
	}
}
