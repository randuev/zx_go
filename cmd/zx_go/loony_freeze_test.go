package main

// loony_freeze_test.go — LOONYFREEZE=1: catch who overwrites the CODE page.
// Hardware + native-emulator reports: after a few frames the screen freezes
// (t stuck ~5) while ISR music plays. Live telnet on the frozen +2 instance
// proved loony.bin byte $8303 ($3C INC A in title's ttli loop) is REPLACED
// at runtime by stamp sprite bytes ($32,$FE,$88...) and title then spins.
// This guard boots by injection (deterministic), installs a WriteObserver over
// the whole code page ($8000-$87FF), and FAILS the moment any write lands in
// the code window — with PC, HL/DE/A, scrbase, curX/curY and swb live.
import (
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestLoonyFreeze(t *testing.T) {
	if os.Getenv("LOONYFREEZE") == "" {
		t.Skip("set LOONYFREEZE=1")
	}
	tapRaw, _ := os.ReadFile("/root/nerve-workspace/demos/loony/loony.tap")
	mapRaw, _ := os.ReadFile("/tmp/loony.sym")
	syms := map[string]uint16{}
	re := regexp.MustCompile(`(\w+): (?:EQU|DEF[SE]) 0x0000([0-9A-Fa-f]{4})`)
	for _, m := range re.FindAllStringSubmatch(string(mapRaw), -1) {
		var v uint64
		fmt.Sscanf(m[2], "%x", &v)
		syms[m[1]] = uint16(v)
	}
	blocks := balParseTapBlocks(t, tapRaw)
	var coreData []byte
	var coreAddr uint16
	for i := range blocks {
		b := blocks[i]
		if b.hdr && b.typ == 3 && trimName(b.name) == "LOONY" && i+1 < len(blocks) {
			coreData, coreAddr = blocks[i+1].data, b.start
		}
	}
	_ = coreAddr
	if coreData == nil {
		t.Fatal("LOONY code block not found")
	}
	prevFlags := cliFlagsActive
	nf := cliFlags{}
	nf.noSound = true
	nf.headless = true
	cliFlagsActive = &nf
	defer func() { cliFlagsActive = prevFlags }()
	emu, err := newEmulator(roms.ModelPlus2)
	if err != nil {
		t.Fatal(err)
	}
	emu.paused.Store(false)
	// NATIVE boot — injection never bites (established). Tape trap + held ENTER.
	if err := loadTapeFile(emu, "/root/nerve-workspace/demos/loony/loony.tap"); err != nil {
		t.Fatal(err)
	}
	installTapeTrap(emu)
	landed := false
	for f := 0; f < 900 && !landed; f++ {
		if f == 40 {
			emu.kbd.PressMatrixKey(6, 0x01, true)
		}
		if f == 80 {
			emu.kbd.PressMatrixKey(6, 0x01, false)
		}
		if f == 160 {
			emu.kbd.PressMatrixKey(6, 0x01, true)
		}
		if f == 200 {
			emu.kbd.PressMatrixKey(6, 0x01, false)
		}
		runOneFrameHeadless(emu, roms.ModelPlus2)
		if emu.mem.Read(0x8000) == 0xF3 {
			landed = true
		}
	}
	if !landed {
		t.Fatal("native land never happened")
	}
	t.Logf("native landed frame %d", 0)
	// verify pristine code bytes before arming
	if b := emu.mem.Read(0x80C0); b != 0xE3 {
		t.Fatalf("pre-run byte $80C0=$%02X (expected $E3 CALL) — already corrupt on land?", b)
	}

	var poisonPC uint16
	var poisonAddr uint16
	var poisonVal byte
	emu.mem.WriteObserver = func(addr uint16, val byte, _ uint16) {
		if addr >= 0x8000 && addr <= 0x87FF && poisonPC == 0 {
			poisonPC = emu.cpu.PC
			poisonAddr = addr
			poisonVal = val
		}
	}
	defer func() { emu.mem.WriteObserver = nil }()

	// native boot timing matters: telnet live repro needs free-running
	// frames (real IRQ cadence), manual stepping never bites.
	mism := 0
	// instruction-granular phase: catch calc_row y-in / HL-out, shift_row HL-in
	var calcSeq [][3]uint16  // y_in, hl_out, ret
	var shiftSeq [][2]uint16 // HL_in, ret
	stepBudget := 0
	inCalc := false
	inShift := false
	var calcY, shiftHL, retAddr uint16
	for fr := 0; fr < 600 && poisonPC == 0; fr++ {
		for stepBudget < 200000 && poisonPC == 0 {
			if emu.cpu.PC == 0x81F1 {
				inCalc = true
				calcY = uint16(emu.cpu.A)
				retAddr = uint16(emu.mem.Read(emu.cpu.SP+1))<<8 | uint16(emu.mem.Read(emu.cpu.SP))
			}
			if emu.cpu.PC == 0x8258 {
				inShift = true
				shiftHL = emu.cpu.HL()
				retAddr = uint16(emu.mem.Read(emu.cpu.SP+1))<<8 | uint16(emu.mem.Read(emu.cpu.SP))
			}
			emu.cpu.StepInstructionWithIRQ()
			stepBudget++
			if inCalc && emu.cpu.PC == retAddr && retAddr != 0 {
				inCalc = false
				if len(calcSeq) < 64 {
					calcSeq = append(calcSeq, [3]uint16{calcY, emu.cpu.HL(), retAddr})
				}
			}
			if inShift && emu.cpu.PC == retAddr && retAddr != 0 {
				inShift = false
				if len(shiftSeq) < 64 {
					shiftSeq = append(shiftSeq, [2]uint16{shiftHL, retAddr})
				}
			}
		}
		runOneFrameHeadless(emu, roms.ModelPlus2)
		b1 := emu.mem.Read(0x80C0)
		b2 := emu.mem.Read(0x80C1)
		if (b1 != 0xE3 || b2 != 0x81) && mism == 0 {
			t.Errorf("CODE PAGE SHIFTED at frame %d: $80C0=$%02X $80C1=$%02X (bin E3 81) t=$%02X ttli=$%02X",
				fr, b1, b2, emu.mem.Read(syms["t"]), emu.mem.Read(syms["ttli"]))
			mism++
		}
	}
	if poisonPC == 0 {
		t.Logf("no code-page write in 8M steps; t=%d ttli=$%02X tx=$%02X",
			emu.mem.Read(syms["t"]), emu.mem.Read(syms["ttli"]), emu.mem.Read(syms["tx"]))
		return
	}
	t.Errorf("CODE PAGE WRITE: $%04X <= $%02X from PC=$%04X", poisonAddr, poisonVal, poisonPC)
	t.Errorf("  AF=$%02X%02X BC=$%04X DE=$%04X HL=$%04X IX=$%04X IY=$%04X SP=$%04X",
		emu.cpu.A, emu.cpu.F, emu.cpu.BC(), emu.cpu.DE(), emu.cpu.HL(), emu.cpu.IX, emu.cpu.IY, emu.cpu.SP)
	t.Errorf("  scrbase=$%02X curX=$%02X curY=$%02X swb=$%02X sht=$%02X bx0=$%02X",
		emu.mem.Read(syms["scrbase"]), emu.mem.Read(syms["curX"]), emu.mem.Read(syms["curY"]),
		emu.mem.Read(syms["swb"]), emu.mem.Read(syms["sht"]), emu.mem.Read(syms["bx0"]))
	t.Errorf("  t=$%02X tx=$%02X ttli=$%02X fcnt=$%02X bbuf=$%02X bbk=$%02X",
		emu.mem.Read(syms["t"]), emu.mem.Read(syms["tx"]), emu.mem.Read(syms["ttli"]),
		emu.mem.Read(syms["fcnt"]), emu.mem.Read(syms["bbuf"]), emu.mem.Read(syms["bbk"]))
	t.Errorf("  mmu slots: $4000=%02X $6000=%02X $8000=%02X $A000=%02X $C000=%02X $E000=%02X",
		emu.mem.GetMMU(2), emu.mem.GetMMU(3), emu.mem.GetMMU(4), emu.mem.GetMMU(5), emu.mem.GetMMU(6), emu.mem.GetMMU(7))
	// disassemble around the writer PC from RAM
	t.Logf("bytes near writer PC:")
	for a := poisonPC & 0xFF80; a < poisonPC+0x40; a += 16 {
		line := ""
		for k := 0; k < 16; k++ {
			line += fmt.Sprintf("%02X ", emu.mem.Read(a+uint16(k)))
		}
		t.Logf("  $%04X: %s", a, line)
	}
}
