package z80

import "testing"

func TestAddHLContext(t *testing.T) {
	// ADD HL,DE alone at various PCs
	for _, pc := range []uint16{0x8000, 0x81CC, 0x8245, 0x8100, 0xA000} {
		cpu, mem := createTestCPU()
		cpu.PC = pc
		cpu.setHL(0x9A80)
		cpu.setDE(0x0020)
		mem.Write(pc, 0x19)
		cpu.StepInstruction()
		t.Logf("alone@%04X -> %04X", pc, cpu.HL())
		cleanupTestROMs("test_roms_z80")
	}
	// with preceding LD HL,nn / LD E,A at $81C8
	cpu, mem := createTestCPU()
	cpu.PC = 0x81C8
	cpu.A = 0x20
	prog := []byte{0x21, 0x80, 0x9A, 0x5F, 0x19}
	for i, b := range prog {
		mem.Write(uint16(0x81C8+i), b)
	}
	cpu.StepInstruction()
	cpu.StepInstruction()
	t.Logf("before add: HL=%04X DE=%04X E=%02X D=%02X", cpu.HL(), cpu.DE(), cpu.E, cpu.D)
	cpu.StepInstruction()
	t.Logf("after add: HL=%04X", cpu.HL())
	cleanupTestROMs("test_roms_z80")
}
