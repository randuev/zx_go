package main

import (
	"fmt"
	"os"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

// boots boardtest.bin and prints ASCII bit-art of the played + hand rows
// to settle sprite-shape disputes with ground-truth bytes.
func TestFieldArtDump(t *testing.T) {
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
	for i, v := range bin {
		emu.mem.Write(0x8000+uint16(i), v)
	}
	emu.cpu.SP = 0xBF00
	emu.cpu.PC = 0x8000
	// watch card3's big-suit column on BOTH pages ($4000 display + $C000 paint):
	// byte CBX+1=21, pixel rows y=98..113
	watchAddr := func(page uint16, y int) uint16 {
		return page + uint16(((y&7)<<8)+((y&0x38)<<2)+((y&0xC0)<<5)) + 21
	}
	type wkey struct {
		page uint16
		y    int
	}
	wprev := map[wkey]byte{}
	for y := 98; y <= 113; y++ {
		wprev[wkey{0x4000, y}] = balPeek(emu, watchAddr(0x4000, y))
		wprev[wkey{0xC000, y}] = balPeek(emu, watchAddr(0xC000, y))
	}
	for i := 0; i < 3000000; i++ {
		if balPeek(emu, emu.cpu.PC) == 0x76 {
			break
		}
		emu.cpu.StepInstruction()
		for y := 98; y <= 113; y++ {
			for _, page := range []uint16{0x4000, 0xC000} {
				k := wkey{page, y}
				v := balPeek(emu, watchAddr(page, y))
				if v != wprev[k] {
					fmt.Printf("WRITE pg$%04X y%03d %02X->%02X pc=$%04X\n", page, y, wprev[k], v, emu.cpu.PC)
					wprev[k] = v
				}
			}
		}
	}
	art := func(y0, y1, x0, x1 int, label string) {
		fmt.Printf("== %s ==\n", label)
		for y := y0; y <= y1; y++ {
			base := uint16(0x4000 + ((y & 7) << 8) + ((y & 0x38) << 2) + ((y & 0xC0) << 5))
			line := ""
			for x := x0; x <= x1; x++ {
				v := balPeek(emu, base+uint16(x/8))
				line += fmt.Sprintf("%c", ".#"[(v>>uint(7-(x&7)))&1])
			}
			fmt.Printf("y%03d %s\n", y, line)
		}
	}
	// played card 2 (J spade @ CBX=16 -> x128) full face
	art(80, 119, 128, 151, "played card2 J? face x128-151")
	// played card 3 (K diamond @ x160)
	art(80, 119, 160, 183, "played card3 K? face x160-183")
	// played card 4 (8 club @ x192)
	art(80, 119, 192, 215, "played card4 8? face x192-215")
	// hand first cards ranks+suits strip
	art(136, 175, 88, 135, "hand slot0/1 strip x88-135")

	// font sheet forensics : what putc6 actually reads for each rank char
	fmt.Printf("== FONT chain ==\n")
	for _, ch := range []byte{'J', 'K', '8', '9', 'A'} {
		ci := balPeek(emu, 0xA30D+uint16(ch-0x20))
		base := 0xA36D + uint16(ci)*2
		lo := balPeek(emu, base)
		hi := balPeek(emu, base+1)
		sheet := uint16(hi)<<8 | uint16(lo)
		fmt.Printf("ch %c ci=%d sheet=$%04X :", ch, ci, sheet)
		for r := 0; r < 7; r++ {
			b0 := balPeek(emu, sheet+uint16(r*2))
			b1 := balPeek(emu, sheet+uint16(r*2+1))
			fmt.Printf(" [%02X %02X]", b0, b1)
		}
		fmt.Printf("\n")
	}
}
