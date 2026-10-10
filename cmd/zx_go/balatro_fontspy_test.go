package main

import (
	"fmt"
	"os"
	"testing"
)

func fmtSpc(b []byte) string {
	s := ""
	for _, v := range b {
		s += fmt.Sprintf("%02X ", v)
	}
	return s
}

// Direct-call the ENGINE's drawstr for "SCORE NEEDED" at band5 x16 on a
// cleared canvas; dump every bitmap byte of the band. Compare vs the unit's
// output for the same call — any difference localises the fault to the
// engine's copy of the routine vs its call context.
func TestBalatroFontSpy(t *testing.T) {
	syms := loadBalatroSyms(t)
	emu := bootBalatro(t, syms)
	// clear bitmap
	for a := 0x4000; a < 0x5800; a++ {
		emu.mem.Write(uint16(a), 0)
	}
	str := syms["STR_NEEDED"]
	cur := emu.cpu.PC
	emu.cpu.SP -= 2
	emu.mem.Write(emu.cpu.SP, byte(cur&0xFF))
	emu.mem.Write(emu.cpu.SP+1, byte(cur>>8))
	emu.cpu.H = byte(str>>8); emu.cpu.L = byte(str&0xFF)
	emu.cpu.B = 5
	emu.cpu.C = 16
	emu.cpu.PC = syms["drawstr"]
	p6L := uint16(syms["p6L"])
	hits := 0
	destdump := ""
	for i := 0; i < 300000 && emu.cpu.PC != cur; i++ {
		emu.cpu.StepInstruction()
		if emu.cpu.PC == p6L {
			hits++
			if hits >= 17 && hits <= 19 {
				destdump += fmt.Sprintf("row2-ish hit%d mem40A2=%02X mem41A2=%02X\n", hits, balPeek(emu, 0x40A2), balPeek(emu, 0x41A2))
			}
			if hits <= 3 {
				destdump += fmt.Sprintf("hit%d HL=$%04X iy=$%04X a=%02X memHL=%02X\n", hits, emu.cpu.HL(), emu.cpu.IY, balPeek(emu, emu.cpu.IY), balPeek(emu, emu.cpu.HL()))
			}
		}
	}
	fmt.Printf("p6L hits=%d (expect 7*12=84)\n%s", hits, destdump)
	if emu.cpu.PC != cur {
		t.Fatalf("drawstr never returned: PC=%04X", emu.cpu.PC)
	}
	// dump band5 rows: for each pixel row r0..7, the 8 bytes cols 2..16
	// dump engine putc6 + drawstr bytes
	for _, lbl := range []string{"putc6", "drawstr"} {
		la := syms[lbl]
		for a := int(la); a < int(la)+16; a += 16 {
			ln := ""
			for i := 0; i < 16; i++ {
				ln += fmt.Sprintf("%02X ", balPeek(emu, uint16(a+i)))
			}
			fmt.Printf("%s@%04X: %s\n", lbl, a, ln)
		}
	}
	// PBAND table contents (band -> screen row base)
	pb := syms["PBAND"]
	for b := range 24 {
		lo := int(balPeek(emu, pb+uint16(b)*2))
		hi := int(balPeek(emu, pb+uint16(b)*2+1))
		fmt.Printf("PBAND[%02d]=$%04X\n", b, hi*256+lo)
	}
	// BX and PHASE14 symbols present?
	fmt.Printf("drawstr=%04X putc6=%04X DSB=%04X DSX=%04X DSHL=%04X BX=%04X PHASE14=%04X\n",
		syms["drawstr"], syms["putc6"], syms["DSB"], syms["DSX"], syms["DSHL"], syms["BX"], syms["PHASE14"])
	// unit bytes from its bin
	ub, err := os.ReadFile("/root/nerve-workspace/demos/balatro/unit/unit.bin")
	if err == nil {
		// unit putc6 $8113 .. : find LD HL,PHASE14 opcode 21 xx xx pattern in unit
		fmt.Printf("unit[0x113:0x123]: %s\n", fmtSpc(ub[0x113:0x123]))
		// engine RAM dump read via balPeek above
	}
	_ = err
	for r := 0; r < 8; r++ {
		line := ""
		for c := 2; c < 20; c++ {
			line += fmt.Sprintf("%02X ", balPeek(emu, uint16(0x40A0+(r<<8)+c)))
		}
		fmt.Printf("b5r%d: %s\n", r, line)
	}
}
