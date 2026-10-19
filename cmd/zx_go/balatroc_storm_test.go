package main

// balatroc_storm_test.go — instruction-level witness of the pass-43
// post-START stack storm: SP collapses $FF3B->$6D69 within one frame
// (37 KB of pushes). Steps the +2 core instruction-by-instruction and
// records a (pc, sp) ring so the recurring recursion cycle prints itself.

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func trimName(s string) string { return strings.TrimRight(s, " ") }


func TestBalatrocStormPlus2(t *testing.T) {
	if os.Getenv("STORM") == "" {
		t.Skip("set STORM=1")
	}
	os.Setenv("ZX_GO_STEP_FRAME", "1")
	tapRaw, err := os.ReadFile("/root/nerve-workspace/demos/balatroc-c/build/board.tap")
	if err != nil {
		t.Fatalf("read tap: %v", err)
	}
	blocks := balParseTapBlocks(t, tapRaw)
	var boardAddr uint16
	var boardData []byte
	for i := range blocks {
		b := blocks[i]
		if b.hdr && b.typ == 3 && trimName(b.name) == "board" && i+1 < len(blocks) {
			boardAddr, boardData = b.start, blocks[i+1].data
		}
	}
	if boardData == nil {
		t.Fatal("board CODE block missing")
	}

	prev := cliFlagsActive
	nf := cliFlags{}
	nf.noSound = true
	nf.headless = true
	cliFlagsActive = &nf
	defer func() { cliFlagsActive = prev }()

	emu, err := newEmulator(roms.ModelPlus2)
	if err != nil {
		t.Fatalf("newEmulator(+2): %v", err)
	}
	// catch EVERY 16-bit stack write on the sentinel words
	var wr [][2]uint16
	var wrS [][2]uint16
	emu.mem.SetAllWriteHook(func(addr uint16, val byte) {
		pc := emu.cpu.PC
		if pc >= 0xD6A0 && pc <= 0xD680 {
			return // legit stores inside _tick ($D6A0, pass-48g)
		}
		if addr >= 0xC000 && addr <= 0xFFF7 && (val == 0x65 || val == 0x6D) {
			other := uint16(0)
			if val == 0x65 {
				other = addr + 1
			} else {
				other = addr - 1
			}
			if emu.mem.Read(other) == 0x6D || val == 0x6D {
				if val == 0x65 && emu.mem.Read(addr+1) == 0x6D {
					wr = append(wr, [2]uint16{pc, addr})
				}
				if val == 0x6D && addr >= 0xC001 && emu.mem.Read(addr-1) == 0x65 {
					wr = append(wr, [2]uint16{pc, addr})
				}
			}
		}
		if addr >= 0xF800 && addr <= 0xF87F {
			wrS = append(wrS, [2]uint16{pc, addr})
		}
	})
	defer emu.mem.SetAllWriteHook(nil)
	emu.paused.Store(false)
	for i := 0; i < 220; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	for i, v := range boardData {
		emu.mem.Write(boardAddr+uint16(i), v)
	}
	sent := uint16(0x7D00)
	emu.mem.Write(sent, 0x76)
	emu.cpu.SP = 0xFF56
	emu.mem.Write(0xFF56, byte(sent))
	emu.mem.Write(0xFF57, byte(sent>>8))
	emu.cpu.PC = boardAddr
	emu.cpu.IFF1, emu.cpu.IFF2 = false, false
	emu.cpu.IM = 1

	// run ~150 frames to settle on the title screen
	tstates := uint64(frameTStatesForModel(roms.ModelPlus2))
	for f := 0; f < 150; f++ {
		target := emu.cpu.Tstates() + tstates
		for emu.cpu.Tstates() < target {
			emu.cpu.StepInstructionWithIRQ()
		}
	}
	// inject '0' (row 4, bit 0) and step-record
	if os.Getenv("LONGHOLD") != "" {
		emu.kbd.PressMatrixKey(4, 0x01, true)
	}
	emu.kbd.PressMatrixKey(4, 0x01, true)
	type rec struct {
		pc uint16
		sp uint16
	}
	var ring []rec
	const RING = 60000
	budget := uint64(tstates) * 120
	target := emu.cpu.Tstates() + budget
	minSP := uint16(0xFFFF)
	for emu.cpu.Tstates() < target {
		emu.cpu.StepInstructionWithIRQ()
		ring = append(ring, rec{emu.cpu.PC, emu.cpu.SP})
		if len(ring) > RING {
			ring = ring[1:]
		}
		if emu.cpu.SP < minSP {
			minSP = emu.cpu.SP
		}
		if len(wr) > 0 {
			for k := range wr {
				fmt.Printf("SENT-WRITER pc=$%04X addr=$%04X\n", wr[k][0], wr[k][1])
			}
			wr = wr[:0]
		}
		if len(wrS) > 0 {
			fmt.Printf("SCRATCH-WRITES n=%d first pc=$%04X\n", len(wrS), wrS[0][0])
			wrS = wrS[:0]
		}
		if (emu.cpu.PC >= 0xC524 && emu.cpu.PC <= 0xC4C0) || emu.cpu.PC == 0xD6A0 || emu.cpu.PC == 0xD62B {
			name := fmt.Sprintf("WATCH@%04X", emu.cpu.PC)
			_ = map[uint16]string{}
			fmt.Printf("MUS-CALL %s sp=$%04X IX=$%04X canary=%02X%02X d0=%02X d1=%02X sp+2=%02X%02X\n",
				name, emu.cpu.SP, emu.cpu.IX,
				emu.mem.Read(0xC524), emu.mem.Read(0xC4AF),
				emu.mem.Read(0xC4B0), emu.mem.Read(0xC4B1),
				emu.mem.Read(emu.cpu.SP+2), emu.mem.Read(emu.cpu.SP+3))
		}
		if emu.cpu.PC == 0xD671 {
			fmt.Printf("C264-WATCH sp=$%04X A=$%02X BC=$%04X HL=$%04X DE=$%04X IX=$%04X IY=$%04X ret@sp=%02X %02X sp-2=%02X %02X sp+2=%02X %02X\n",
				emu.cpu.SP, emu.cpu.A, emu.cpu.BC(), emu.cpu.HL(), emu.cpu.DE(), emu.cpu.IX, emu.cpu.IY,
				emu.mem.Read(emu.cpu.SP), emu.mem.Read(emu.cpu.SP+1),
				emu.mem.Read(emu.cpu.SP-2), emu.mem.Read(emu.cpu.SP-1),
				emu.mem.Read(emu.cpu.SP+2), emu.mem.Read(emu.cpu.SP+3))
		}
		if emu.cpu.PC < 0x4000 {
			break // ROM takeover — the crash
		}
	}
	fmt.Printf("STORM: minSP=$%04X recorded=%d\n", minSP, len(ring))
	// print the recurring cycle: find the most frequent PC in the ring
	freq := map[uint16]int{}
	for _, r := range ring {
		freq[r.pc]++
	}
	type pf struct {
		pc  uint16
		n   int
	}
	var top []pf
	for pc, n := range freq {
		top = append(top, pf{pc, n})
	}
	for i := 0; i < len(top); i++ {
		for j := i + 1; j < len(top); j++ {
			if top[j].n > top[i].n {
				top[i], top[j] = top[j], top[i]
			}
		}
	}
	for k := 0; k < len(top) && k < 15; k++ {
		fmt.Printf("  PC=$%04X x%d\n", top[k].pc, top[k].n)
	}
	// tail trace — recursion cycle prints itself
	fmt.Println("  --- tail 40 ---")
	t0 := len(ring) - 40
	if t0 < 0 {
		t0 = 0
	}
	for _, r := range ring[t0:] {
		fmt.Printf("  $%04X sp=$%04X\n", r.pc, r.sp)
	}
	emu.kbd.PressMatrixKey(4, 0x01, false)
}
