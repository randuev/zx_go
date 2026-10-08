package main

import (
	"fmt"
	"testing"
)

func TestBalatroPutcProbe(t *testing.T) {
	syms := loadBalatroSyms(t)
	emu := bootBalatro(t, syms)
	// step until first putc entry; log DE/A; run one putc; read cell back
	pcPutc := syms["putc"]
	found := false
	var de0 uint16; var a0 byte
	for i := 0; i < 2000000; i++ {
		if emu.cpu.PC == pcPutc {
			de0 = emu.cpu.DE()
			a0 = emu.cpu.A
			found = true
			break
		}
		emu.cpu.StepInstructionWithIRQ()
	}
	if !found {
		t.Fatal("putc never called in 2M steps")
	}
	fmt.Printf("putc#1 A=%02X DE=%04X\n", a0, de0)
	for i := 0; i < 40; i++ {
		op := balPeek(emu, emu.cpu.PC)
		fmt.Printf("putcT%02d pc=%04X op=%02X DE=%04X HL=%04X\n", i, emu.cpu.PC, op, emu.cpu.DE(), emu.cpu.HL())
		emu.cpu.StepInstruction()
	}
	line := ""
	for r := 0; r < 8; r++ {
		b := balPeek(emu, de0+uint16(r*0x100))
		line += fmt.Sprintf("%02X ", b)
	}
	fmt.Printf("cell bytes after putc: %s\n", line)
	balRunFrames(emu, 2)
	d0 := balPeek(emu, syms["DEFTAB"]) | balPeek(emu, syms["DEFTAB"]+1)<<8
	fmt.Printf("DEFTAB0=%04X\n", d0)
	// peek first 32 bytes of screen row0 block0 and row1
	for _, a := range []uint16{0x4000, 0x4008, 0x4100, 0x4108} {
		lb := ""
		for i := 0; i < 8; i++ {
			b := balPeek(emu, a+uint16(i))
			lb += fmt.Sprintf("%02X ", b)
		}
		fmt.Printf("%04X: %s\n", a, lb)
	}
	// font 'B' bytes at FONT+2*8
	f := syms["FONT"]
	lb := ""
	for i := 0; i < 8; i++ {
		lb += fmt.Sprintf("%02X ", balPeek(emu, f+2+uint16(i)))
	}
	fmt.Printf("FONT+2*8 ('B'): %s\n", lb)
	reg := emu.mem.RAM8KPage(10)
	hits := map[uint32]int{}
	for i, b := range reg[:0x1800] {
		if b != 0 {
			a := uint32(0x4000 + i)
			hits[uint32(a&0xFF00)]++
		}
	}
	for a, n := range hits {
		fmt.Printf("lit page%04X: %d\n", a, n)
	}
	lb = ""
	for r := 0; r < 24; r++ {
		lo := balPeek(emu, syms["DEFTAB"]+uint16(r*2))
		hi := balPeek(emu, syms["DEFTAB"]+uint16(r*2+1))
		lb += fmt.Sprintf("%02X%02X ", hi, lo)
	}
	fmt.Printf("DEFTAB: %s\n", lb)
}
