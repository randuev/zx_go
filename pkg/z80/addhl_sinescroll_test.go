package z80

import "testing"

// TestAddHLDE_SinescrollRepro — the exact sequence from sinescroll gloop:
// ld hl,$9A80 / ld e,a / add hl,de / ld a,(hl). Observed live HL wrong
// ($9A80+$20 -> $9BA0) inside the full emulator.
func TestAddHLDE_SinescrollRepro(t *testing.T) {
	cpu, mem := createTestCPU()
	cpu.PC = 0x81C8
	cpu.A = 0x20
	cpu.D = 0x00 // harness powers on DE=$FFFF; sinescroll's live DE was a clean 16-bit offset
	prog := []byte{0x21, 0x80, 0x9A, 0x5F, 0x19, 0x7E}
	for i, b := range prog {
		mem.Write(uint16(0x81C8+i), b)
	}
	mem.Write(0x9AA0, 0xFF)
	for _, want := range []uint16{0x9A80, 0x9A80, 0x9AA0} {
		cpu.StepInstruction()
		t.Logf("PC=$%04X H=%02X L=%02X D=%02X E=%02X", cpu.PC, cpu.H, cpu.L, cpu.D, cpu.E)
		if cpu.HL() != want {
			t.Fatalf("HL=$%04X want $%04X after opcode at PC=%04X", cpu.HL(), want, cpu.PC)
		}
	}
	cpu.StepInstruction() // ld a,(hl)
	if cpu.A != 0xFF {
		t.Fatalf("A=$%02X want FF", cpu.A)
	}
	cleanupTestROMs("test_roms_z80")
}
