package main

// balatroc_diag_test.go — pass 48 EXIT hunting. The game runs from $8000
// then exits into the 128 editor key-wait ($36A9/$36AB, SP=$5BFB). Step
// every instruction; break at the first PC below $8000; dump the last 40
// in-game trajectory + the stack window so we can see the bad RET/JUMP.

import (
	"fmt"
	"os"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestBalatrocDiagExit48(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	tapRaw, _ := os.ReadFile("/root/nerve-workspace/demos/balatroc-c/build/board.tap")
	blocks := balParseTapBlocks(t, tapRaw)
	var a uint16
	var bd []byte
	for i := range blocks {
		b := blocks[i]
		if b.hdr && b.typ == 3 && trimName(b.name) == "core" && i+1 < len(blocks) {
			a, bd = b.start, blocks[i+1].data
		}
	}
	prev := cliFlagsActive
	nf := cliFlags{}
	nf.headless = true
	cliFlagsActive = &nf
	defer func() { cliFlagsActive = prev }()
	emu, err := newEmulator(roms.ModelPlus2)
	if err != nil {
		t.Fatal(err)
	}
	emu.paused.Store(true)
	for i := 0; i < 220; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	for i, v := range bd {
		emu.mem.Write(a+uint16(i), v)
	}
	sent := uint16(0x7D00)
	emu.mem.Write(sent, 0x76)
	emu.cpu.SP = 0xBFFE
	emu.mem.Write(0xFFFC, byte(sent))
	emu.mem.Write(0xFFFD, byte(sent>>8))
	emu.cpu.PC = 0x8000
	emu.cpu.Halted = false
	emu.cpu.IFF1, emu.cpu.IFF2 = false, false
	emu.cpu.IM = 1
	fmt.Printf("HIJACK pc=$%04X sp=$%04X at=$%04X bd=%d\n", emu.cpu.PC, emu.cpu.SP, a, len(bd))
	type rec struct {
		pc uint16
		sp uint16
		a  byte
		h  uint16
	}
	var hist []rec
	exited := false
	for steps := 0; steps < 900000; steps++ {
		if emu.cpu.Halted {
			break
		}
		pc := emu.cpu.PC
		hist = append(hist, rec{pc, emu.cpu.SP, emu.cpu.A, emu.cpu.HL()})
		if pc < 0x8000 {
			exited = true
			fmt.Printf("EXIT at step %d: PC=$%04X SP=$%04X\n", steps, pc, emu.cpu.SP)
			break
		}
		if pc == 0x8041 {
			prev := hist[len(hist)-2].pc
			fmt.Printf("EXIT-ENTRY step %d prev=$%04X prevbytes=", steps, prev)
			for j := prev; j < prev+4; j++ {
				fmt.Printf(" %02X", emu.mem.Read(j))
			}
			fmt.Printf(" SP=$%04X HL=$%04X stk=", emu.cpu.SP, emu.cpu.HL())
			for j := emu.cpu.SP; j < emu.cpu.SP+8; j++ {
				fmt.Printf(" %02X", emu.mem.Read(j))
			}
			fmt.Println()
			break
		}
		emu.cpu.StepInstructionWithIRQ()
	}
	for i := 0; i < len(hist); i++ {
		h := hist[i]
		fmt.Printf("%6d PC=$%04X SP=$%04X A=$%02X HL=$%04X\n", i, h.pc, h.sp, h.a, h.h)
	}
	for j := uint16(0x8000); j < 0x8052; j += 16 {
		fmt.Printf("CRT $%04X:", j)
		for k := j; k < j+16 && k < 0x8052; k++ {
			fmt.Printf(" %02X", emu.mem.Read(k))
		}
		fmt.Println()
	}
	if exited {
		for j := emu.cpu.SP; j < emu.cpu.SP+24; j++ {
			fmt.Printf("  stk $%04X=$%02X\n", j, emu.mem.Read(j))
		}
	}
}
