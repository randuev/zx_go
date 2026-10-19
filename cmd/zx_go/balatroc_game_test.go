package main

// TestBalatrocGameMusicPlus2 — pass 43 end-to-end proof.
// Boots the integrated board.tap on a +2 core, loads BOTH CODE blocks via
// the logical memory (paging handled by the emulator), records the AY mix,
// drives real key strokes through the matrix (bindings from input.c), and
// samples _qctr per frame to measure the quark rate for calibration.
// Ground-truth deliverable: the WAV (FFT external) + quarks/frame print.

import (
	"encoding/binary"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func balParseTapBlocks(t *testing.T, tap []byte) []struct {
	hdr  bool
	typ   byte
	name  string
	start uint16
	data  []byte
} {
	t.Helper()
	var out []struct {
		hdr  bool
		typ   byte
		name  string
		start uint16
		data  []byte
	}
	o := 0
	for o+2 <= len(tap) {
		ln := int(binary.LittleEndian.Uint16(tap[o:]))
		o += 2
		body := tap[o : o+ln]
		o += ln
		var xors byte
		for _, b := range body[:len(body)-1] {
			xors ^= b
		}
		if xors != body[len(body)-1] {
			t.Fatalf("tap block @%d checksum", o)
		}
		if body[0] == 0 {
			out = append(out, struct {
				hdr  bool
				typ   byte
				name  string
				start uint16
				data  []byte
			}{true, body[1], string(body[2:12]), binary.LittleEndian.Uint16(body[14:]), nil})
		} else {
			out = append(out, struct {
				hdr  bool
				typ   byte
				name  string
				start uint16
				data  []byte
			}{false, 0, "", 0, body[1 : len(body)-1]})
		}
	}
	return out
}

func TestBalatrocGameMusicPlus2(t *testing.T) {
	wavPath := os.Getenv("BALATROC_WAV")
	if wavPath == "" {
		wavPath = "/tmp/balatroc_game.wav"
	}
	tapRaw, err := os.ReadFile("/root/nerve-workspace/demos/balatroc-c/build/board.tap")
	if err != nil {
		t.Fatalf("read tap: %v", err)
	}
	blocks := balParseTapBlocks(t, tapRaw)
	var boardAddr uint16
	var boardData, musicData []byte
	for i := range blocks {
		b := blocks[i]
		if b.hdr && b.typ == 3 && strings.TrimRight(b.name, " ") == "board" && i+1 < len(blocks) {
			boardAddr, boardData = b.start, blocks[i+1].data
		}
		if b.hdr && b.typ == 3 && strings.TrimRight(b.name, " ") == "music" && i+1 < len(blocks) {
			musicData = blocks[i+1].data
		}
	}
	// pass-48: bulba retired — the AY player is EMBEDDED in the board block;
	// there is no second CODE block any more. Only board must exist.
	if boardData == nil {
		t.Fatal("board CODE block missing")
	}

	prev := cliFlagsActive
	nf := cliFlags{}
	nf.noSound = false
	nf.headless = true
	nf.recordAudio = wavPath
	cliFlagsActive = &nf
	defer func() { cliFlagsActive = prev }()

	emu, err := newEmulator(roms.ModelPlus2)
	if err != nil {
		t.Fatalf("newEmulator(+2): %v", err)
	}
	emu.paused.Store(false)
	for i := 0; i < 220; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	maybeStartAudioRecording(emu.ula)

	// load both CODE blocks through the LOGICAL memory — the +2 core pages
	// writes into bank 2 / the paged upper slot automatically
	for i, v := range boardData {
		emu.mem.Write(boardAddr+uint16(i), v)
	}
	_ = musicData // pass-48: unused — no $6000 PT3 stage any more
	if emu.mem.Read(0x8000) == 0 {
		t.Fatal("board not loaded")
	}

	// RANDOMIZE USR contract: fresh stack, return sentinel, DI law.
	// Sentinel OUTSIDE board image ($8000-$F87F). SP at RAMTOP $FF57
	// like real BASIC USR — a low stack ($7D80) descended into the PT3
	// module at $6000-$6F47, zeroed it and ret-popped garbage to ROM.
	sent := uint16(0x7D00)
	emu.mem.Write(sent, 0x76)
	emu.cpu.SP = 0xFF57
	emu.mem.Write(0xFF56, byte(sent))
	emu.mem.Write(0xFF57, byte(sent>>8))
	emu.cpu.SP = 0xFF56
	emu.cpu.PC = boardAddr
	emu.cpu.IFF1, emu.cpu.IFF2 = false, false
	emu.cpu.IM = 1

	const qctr = 0xfd1d
	const sfxID = 0xfd1f
	quarks, frames := 0, 0
	lastGamePC := uint16(0)
	romTaken := false
	spPrev := uint16(0)
	prevQ := uint16(emu.mem.Read(qctr))|uint16(emu.mem.Read(qctr+1))<<8
	// key bindings (input.c): LEFT=(3,0x10) RIGHT=(4,0x04) SEL=(4,0x08)
	// PLAY='0'=(4,0x01) DISC=(4,0x10)
	runF := func(n int) {
		for i := 0; i < n; i++ {
			runOneFrameHeadless(emu, roms.ModelPlus2)
			emu.ula.PumpAudioFrame()
			q := uint16(emu.mem.Read(qctr))|uint16(emu.mem.Read(qctr+1))<<8
			if q > prevQ {
				quarks++
			}
			prevQ = q
			frames++
			// crash witness: last PC inside game memory before ROM takes over
			pc := emu.cpu.PC
			if pc >= 0x4000 && pc < 0x8000 {
				// bank-5 (screen/music) — also "alive" territory
			} else if pc >= 0x8000 {
				lastGamePC = pc
			} else if pc < 0x4000 && !romTaken {
				romTaken = true
				sp := emu.cpu.SP
				fmt.Printf("  !! ROM takeover at frame %d — last game PC=$%04X SP=$%04X stack=%02X %02X %02X %02X %02X\n",
					frames, lastGamePC, sp,
					emu.mem.Read(sp), emu.mem.Read(sp+1), emu.mem.Read(sp+2), emu.mem.Read(sp+3), emu.mem.Read(sp+4))
			}
			if frames%250 == 0 {
				fmt.Printf("  f=%-4d PC=$%04X SP=$%04X qctr=%d\n", frames, emu.cpu.PC, emu.cpu.SP, q)
			}
			if spPrev != 0 && int(spPrev)-int(emu.cpu.SP) > 4 {
				fmt.Printf("  STORM f=%d SP $%04X->$%04X PC=$%04X\n", frames, spPrev, emu.cpu.SP, emu.cpu.PC)
			}
			spPrev = emu.cpu.SP
		}
	}
	stroke := func(row byte, bit byte, tail int) {
		emu.kbd.PressMatrixKey(int(row), bit, true)
		runF(tail)
		emu.kbd.PressMatrixKey(int(row), bit, false)
		runF(200)
	}

	_ = stroke
	runF(150)                 // title screen, music breathing
	stroke(4, 0x01, 400)    // '0' = START from title menu
	stroke(4, 0x04, 200)    // RIGHT
	stroke(4, 0x08, 200)    // SEL
	stroke(4, 0x08, 200)    // SEL again
	stroke(4, 0x04, 200)    // RIGHT
	stroke(4, 0x10, 400)    // DISC

	if err := emu.ula.StopRecording(); err != nil {
		t.Fatalf("StopRecording: %v", err)
	}
	fmt.Printf("BALATROC: frames=%d quarks=%d qpf=%.2f sfxID=%d PC=$%04X\n",
		frames, quarks, float64(quarks)/float64(frames), emu.mem.Read(sfxID), emu.cpu.PC)
}
