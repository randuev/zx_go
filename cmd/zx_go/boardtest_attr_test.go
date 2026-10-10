package main

import (
	"fmt"
	"os"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

// boots boardtest.bin to halt, then dumps attr grid + per-card bitmap census.
func TestBoardTestAttrCensus(t *testing.T) {
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
	fmt.Printf("pre-run y84bx13=%02X\n", balPeek(emu, uint16(0x4C40+13)))
	halted := false
	// watch target sprite-col bytes for every change; log the executing insn (pc_backtrack via pre-step pc), opcode, A/DE/HL
	type wr struct {
		pc, op, hl, de, before, after uint16
	}
	var wrs []wr
	targets := map[uint16]bool{}
	for r := 72; r < 104; r++ {
		y := r
		base := uint16(0x4000 + ((y & 7) << 8) + ((y & 0x38) << 2) + ((y & 0xC0)<<5)) + 13
		targets[base] = true
	}
	for i := 0; i < 3000000 && !halted; i++ {
		if balPeek(emu, emu.cpu.PC) == 0x76 {
			halted = true
			break
		}
		// snapshot target bytes
		var prev map[uint16]byte = map[uint16]byte{}
		for t := range targets {
			prev[t] = balPeek(emu, t)
		}
		pc0 := emu.cpu.PC
		op := uint16(balPeek(emu, pc0))
		emu.cpu.StepInstruction()
		if len(wrs) < 60 {
			for t := range targets {
				nv := balPeek(emu, t)
				if nv != prev[t] {
					wrs = append(wrs, wr{pc0, op, emu.cpu.HL(), emu.cpu.DE(), uint16(prev[t]), uint16(nv)})
				}
			}
		}
		if i == 20000 {
			b := uint16(0x4C40 + 13)
			fmt.Printf("mid i=%d y84bx13=%02X pc=$%04X\n", i, balPeek(emu, b), emu.cpu.PC)
		}
	}
	b84 := uint16(0x4C40 + 13)
	fmt.Printf("at-halt y84bx13=%02X wrs=%d\n", balPeek(emu, b84), len(wrs))
	if len(wrs) > 0 {
		fmt.Printf("WRITES into played sprite col bx13 y84..91:\n")
		for _, w := range wrs {
			fmt.Printf("pc=$%04X op=%02X hl=$%04X de=$%04X %02X->%02X\n", w.pc, w.op, w.hl, w.de, w.before, w.after)
		}
	}
	fmt.Printf("halted=%v pc=$%04X\n", halted, emu.cpu.PC)
	// attr grid b0..23 x col0..31
	fmt.Printf("    ")
	for c := 0; c < 32; c++ {
		fmt.Printf("%X", c%16)
	}
	fmt.Printf("\n")
	for b := 0; b < 24; b++ {
		fmt.Printf("b%02d ", b)
		for c := 0; c < 32; c++ {
			fmt.Printf("%02X", balPeek(emu, uint16(0x5800+b*32+c)))
		}
		fmt.Printf("\n")
	}
	// bitmap lit-pixel census per band over card cols
	for b := 8; b < 21; b++ {
		lit := 0
		for c := 11; c < 32; c++ {
			for r := 0; r < 8; r++ {
				y := b*8 + r
				a := uint16(0x4000 + ((y & 7) << 8) + ((y & 0x38) << 2) + ((y & 0xC0) << 5) + c)
				v := balPeek(emu, a)
				for bt := 0; bt < 8; bt++ {
					if (v>>uint(bt))&1 == 1 {
						lit++
					}
				}
			}
		}
		fmt.Printf("bitmap b%02d cols11-31 lit=%d\n", b, lit)
	}
	// ---- sprite forensics ----
	// SUIT8 table bytes ($889E.., 4 suits x 8)
	fmt.Printf("SUIT8 table:\n")
	for s := 0; s < 4; s++ {
		line := ""
		for i := 0; i < 8; i++ {
			line += fmt.Sprintf("%02X ", balPeek(emu, uint16(0x889E+s*8+i)))
		}
		fmt.Printf("s%d: %s\n", s, line)
	}
	// played card0 (H, BX=13, Y=84) : every byte of the sprite rect on screen
	fmt.Printf("played sprite rect bx13 y84..91:\n")
	for r := 84; r < 100; r++ {
		a := uint16(0x4000+((r&7)<<8)+((r&0x38)<<2)+((r&0xC0)<<5)) + 12
		fmt.Printf("y%03d:", r)
		for c := 12; c < 15; c++ {
			fmt.Printf(" %02X", balPeek(emu, a+uint16(c)))
		}
		fmt.Printf("\n")
	}
	// hand card0 (S, BX=12, Y=148)
	fmt.Printf("hand sprite rect bx12 y148..155:\n")
	for r := 148; r < 156; r++ {
		a := uint16(0x4000+((r&7)<<8)+((r&0x38)<<2)+((r&0xC0)<<5)) + 11
		fmt.Printf("y%03d:", r)
		for c := 11; c < 14; c++ {
			fmt.Printf(" %02X", balPeek(emu, a+uint16(c)))
		}
		fmt.Printf("\n")
	}
	// bit-art of played card0 face : cols 12..15 bytes, rows 72..103
	fmt.Printf("card0 face bitart (bx12..15, y72..103):\n")
	for r := 72; r < 104; r++ {
		base := uint16(0x4000 + ((r & 7) << 8) + ((r & 0x38) << 2) + ((r & 0xC0) << 5))
		line := ""
		for c := 12; c < 16; c++ {
			v := balPeek(emu, base+uint16(c))
			for b := 7; b >= 0; b-- {
				if (v>>uint(b))&1 == 1 {
					line += "#"
				} else {
					line += "."
				}
			}
		}
		fmt.Printf("y%03d %s\n", r, line)
	}
	// sheet glyph bit-art for suspect ci : J(9) K(10) 1(17) 8(24) 9(25)
	fmt.Printf("sheet glyph bit-art:\n")
	for _, ci := range []int{26, 27, 28, 29, 30} {
		lo := uint16(balPeek(emu, 0xA23F+uint16(ci)*2))
		hi := uint16(balPeek(emu, 0xA23F+uint16(ci)*2+1))
		p := hi<<8 | lo
		fmt.Printf("ci%02d @$%04X:\n", ci, p)
		for r := 0; r < 8; r++ {
			b0 := balPeek(emu, p+uint16(r)*2)
			b1 := balPeek(emu, p+uint16(r)*2+1)
			line := ""
			for b := 7; b >= 2; b-- {
				if (b0>>uint(b))&1 == 1 {
					line += "#"
				} else {
					line += "."
				}
			}
			for b := 7; b >= 2; b-- {
				if (b1>>uint(b))&1 == 1 {
					line += "#"
				} else {
					line += "."
				}
			}
			fmt.Printf("  %s\n", line)
		}
	}
	// font index chain for 'J' (rank) : IDX[0x2A] -> CI[n] -> sheet bytes
	{
		n := balPeek(emu, uint16(0xA1DF+0x2A)) // 'J' - ' '
		fmt.Printf("FONT6IDX['J']=%d\n", n)
		lo := uint16(balPeek(emu, 0xA23F+uint16(n)*2))
		hi := uint16(balPeek(emu, 0xA23F+uint16(n)*2+1))
		p := hi<<8 | lo
		fmt.Printf("FONT6CI[%d]=$%04X bytes:", n, p)
		for i := 0; i < 8; i++ {
			fmt.Printf(" %02X", balPeek(emu, p+uint16(i)))
		}
		fmt.Printf("\n")
		// 'K' (ascii 0x4B idx 43)
		nk := balPeek(emu, uint16(0xA1DF+0x2B))
		fmt.Printf("FONT6IDX['K']=%d\n", nk)
		lo2 := uint16(balPeek(emu, 0xA23F+uint16(nk)*2))
		hi2 := uint16(balPeek(emu, 0xA23F+uint16(nk)*2+1))
		p2 := hi2<<8 | lo2
		fmt.Printf("FONT6CI[%d]=$%04X bytes:", nk, p2)
		for i := 0; i < 8; i++ {
			fmt.Printf(" %02X", balPeek(emu, p2+uint16(i)))
		}
		fmt.Printf("\n")
	}
	// ROWTAB sanity vs canonical ULA formula
	fmt.Printf("ROWTAB check:\n")
	for _, y := range []int{0, 84, 85, 87, 88, 91, 92, 127, 128, 148, 191} {
		lo := uint16(balPeek(emu, uint16(0x88BE+y*2)))
		hi := uint16(balPeek(emu, uint16(0x88BE+y*2+1)))
		rt := hi<<8 | lo
		canon := uint16(0x4000 + ((y & 7) << 8) + ((y & 0x38) << 2) + ((y & 0xC0) << 5))
		flag := "OK"
		if rt != canon {
			flag = "**MISMATCH**"
		}
		fmt.Printf("y%03d ROWTAB=$%04X canon=$%04X %s\n", y, rt, canon, flag)
	}
}
