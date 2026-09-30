package main

// Throwaway: pump frames; hook every fetch; dump the exact moment execution
// leaves the code window, with a 200-deep trail.

import (
	"fmt"
	"os"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestSnowEscProbe(t *testing.T) {
	prev := cliFlagsActive
	nf := cliFlags{}
	if prev != nil {
		nf = *prev
	}
	nf.noSound = true
	cliFlagsActive = &nf
	defer func() { cliFlagsActive = prev }()

	raw, err := os.ReadFile("/root/nerve-workspace/demos/snow/snow.tap")
	if err != nil {
		t.Fatal(err)
	}
	var code []byte
	off := 0
	var pend uint16
	for off+2 <= len(raw) {
		n := int(raw[off]) | int(raw[off+1])<<8
		off += 2
		p := raw[off : off+n]
		off += n
		if len(p) < 16 {
			continue
		}
		if p[0] == 0 && p[1] == 3 {
			pend = uint16(p[14]) | uint16(p[15])<<8
			continue
		}
		if p[0] == 0xFF && pend != 0 {
			code = p[1 : n-1]
		}
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

	var ring []string
	dumped := false
	emu.cpu.AddPreFetchHook("esc", func(pc uint16) {
		if dumped {
			return
		}
		if pc < 0x8000 || pc > 0x8BFF {
			dumped = true
			for _, r := range ring {
				fmt.Println(r)
			}
			fmt.Printf("ESCAPED pc=%04X sp=%04X A=%02X F=%02X B=%02X C=%02X D=%02X E=%02X H=%02X L=%02X ix=%04X iy=%04X im=%d iff=%v\n",
				pc, emu.cpu.SP, emu.cpu.A, emu.cpu.F, emu.cpu.B, emu.cpu.C, emu.cpu.D, emu.cpu.E, emu.cpu.H, emu.cpu.L, emu.cpu.IX, emu.cpu.IY, emu.cpu.IM, emu.cpu.IFF1)
			fmt.Printf("stack@SP: %02X %02X %02X %02X %02X %02X %02X %02X\n",
				emu.mem.Read(emu.cpu.SP), emu.mem.Read(emu.cpu.SP+1), emu.mem.Read(emu.cpu.SP+2), emu.mem.Read(emu.cpu.SP+3),
				emu.mem.Read(emu.cpu.SP+4), emu.mem.Read(emu.cpu.SP+5), emu.mem.Read(emu.cpu.SP+6), emu.mem.Read(emu.cpu.SP+7))
			fmt.Printf("B0FF,B100,B101: %02X %02X %02X | 898D,898E,898F,8990: %02X %02X %02X %02X\n",
				emu.mem.Read(0xB0FF), emu.mem.Read(0xB100), emu.mem.Read(0xB101),
				emu.mem.Read(0x898D), emu.mem.Read(0x898E), emu.mem.Read(0x898F), emu.mem.Read(0x8990))
			return
		}
		ring = append(ring, fmt.Sprintf("pc=%04X sp=%04X hl=%02X:%02X", pc, emu.cpu.SP, emu.cpu.H, emu.cpu.L))
		if len(ring) > 200 {
			ring = ring[1:]
		}
	})
	for fr := 0; fr < 8 && !dumped; fr++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	if !dumped {
		fmt.Println("NO ESCAPE in 8 frames")
	}
}
