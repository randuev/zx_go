package z80

import "testing"

func TestAddHLMatrix(t *testing.T) {
	cases := []struct {
		hl, de uint16
	}{
		{0x9A80, 0x0020}, {0x0001, 0x0001}, {0x0080, 0x0080}, {0x00FF, 0x0001},
		{0x1234, 0x5678}, {0x8000, 0x0001}, {0x9A80, 0x0002}, {0x9A80, 0x0001},
		{0x9A80, 0x0010}, {0x9A80, 0x0040}, {0x9A80, 0x0080}, {0x9A80, 0x0100},
		{0x9A00, 0x0080}, {0x9A80, 0x0008}, {0x9A80, 0x0004}, {0x9A80, 0x0000},
		{0x9B80, 0x0020}, {0x9B00, 0x0020}, {0x9A80, 0x0021}, {0x9A80, 0x0022},
	}
	for _, c := range cases {
		cpu, mem := createTestCPU()
		cpu.PC = 0x8000
		cpu.setHL(c.hl)
		cpu.setDE(c.de)
		cpu.WZ = 0
		// opcode 0x19 lives at 0x8000 via mem not writable? createTestCPU ram window includes 0x8000
		mem.Write(0x8000, 0x19)
		cpu.StepInstruction()
		got := cpu.HL()
		want := c.hl + c.de
		mark := "ok"
		if got != want {
			mark = "WRONG"
		}
		t.Logf("%04X+%04X=%04X got %04X %s", c.hl, c.de, want, got, mark)
		cleanupTestROMs("test_roms_z80")
	}
}
