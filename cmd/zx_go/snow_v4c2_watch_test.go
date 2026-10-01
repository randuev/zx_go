package main

// Throwaway: run v4c2 tap live, find where PC wedges or escapes.

import (
	"fmt"
	"os"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestSnowV4c2Watch(t *testing.T) {
	prev := cliFlagsActive
	nf := cliFlags{}
	if prev != nil {
		nf = *prev
	}
	nf.noSound = true
	cliFlagsActive = &nf
	defer func() { cliFlagsActive = prev }()

	tapPath := "/root/nerve-workspace/demos/snow/snow.tap"
	if p := os.Getenv("SNOW_TAP"); p != "" {
		tapPath = p
	}
	raw, err := os.ReadFile(tapPath)
	if err != nil {
		t.Fatal(err)
	}
	var code []byte
	off := 0
	for off+2 <= len(raw) {
		n := int(raw[off]) | int(raw[off+1])<<8
		off += 2
		p := raw[off : off+n]
		off += n
		if len(p) >= 16 && p[0] == 0xFF && n > 5000 {
			code = p[1 : n-1]
		}
	}
	if code == nil {
		t.Fatal("no CODE block")
	}
	emu, err := newEmulator(roms.ModelPlus2)
	if err != nil {
		t.Fatal(err)
	}
	emu.paused.Store(false)
	for i := 0; i < 220; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	for i, v := range code {
		emu.mem.Write(uint16(0x8000+i), v)
	}
	emu.cpu.SP = 0xFF00
	emu.mem.Write(0xFEFF, 0x00)
	emu.mem.Write(0xFEFE, 0x00)
	emu.cpu.PC = 0x8000
	emu.cpu.IFF1, emu.cpu.IFF2 = false, false
	emu.cpu.IM = 1

	var hist []uint16
	badPC := uint16(0xFFFF)
	badFrame := -1
	emu.cpu.AddPreFetchHook("watch", func(pc uint16) {
		if badPC != 0xFFFF {
			return
		}
		if pc < 0x8000 || pc > 0x8BFF {
			badPC = pc
			badFrame = int(emu.mem.Read(0x8C91)) | int(emu.mem.Read(0x8C92))<<8
			return
		}
		hist = append(hist, pc)
		if len(hist) > 60 {
			hist = hist[len(hist)-60:]
		}
	})
	n := 600
	spMin := uint16(0xFFFF)
	for f := 0; f < n; f++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
		if emu.cpu.SP < spMin {
			spMin = emu.cpu.SP
		}
		if f%100 == 0 {
			fmt.Printf("f=%d PC=$%04X SP=$%04X halted=%v sval=%d\n",
				f, emu.cpu.PC, emu.cpu.SP, emu.cpu.Halted, emu.mem.Read(0x8CA0))
		}
		if badPC != 0xFFFF {
			fmt.Printf("BAD PC $%04X at fcnt=%d f=%d\nprev60: ", badPC, badFrame, f)
			for _, p := range hist {
				fmt.Printf("$%04X ", p)
			}
			fmt.Println()
			fmt.Printf("SP=$%04X IM=%d IFF1=%v IFF2=%v AF=$%04X\n",
				emu.cpu.SP, emu.cpu.IM, emu.cpu.IFF1, emu.cpu.IFF2, uint16(emu.cpu.A)<<8|uint16(emu.cpu.F))
			fmt.Printf("stack@SP: ")
			for k := 0; k < 8; k++ {
				fmt.Printf("%02x ", emu.mem.Read(uint16(int(emu.cpu.SP)+k)))
			}
			fmt.Println()
			return
		}
	}
	fmt.Printf("no escape: last PC=$%04X halt=%v sval=%d sroll=%d\n",
		emu.cpu.PC, emu.cpu.Halted, emu.mem.Read(0x8CA0),
		int(emu.mem.Read(0x8C9E))|int(emu.mem.Read(0x8C9F))<<8)
}
