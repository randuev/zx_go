package main

// One-shot probe (delete when done): catch every write of a byte with the
// FLASH bit (0x80) into either attribute page, logging value + authoring PC,
// plus zero the CF strip at load time and confirm nothing rewrites it.

import (
	"encoding/binary"
	"fmt"
	"os"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestCruxayCFProbe(t *testing.T) {
	tapPath := "/root/nerve-workspace/demos/cruxay/cruxay_v20h.tap"
	if p := os.Getenv("CRUXAY_TAP"); p != "" {
		tapPath = p
	}
	raw, err := os.ReadFile(tapPath)
	if err != nil {
		t.Fatalf("read tap: %v", err)
	}
	var blk []byte
	var loadAddr uint16
	off := 0
	for off+2 <= len(raw) {
		n := int(binary.LittleEndian.Uint16(raw[off:]))
		off += 2
		if off+n > len(raw) {
			break
		}
		payload := raw[off : off+n]
		off += n
		if len(payload) < 16 {
			continue
		}
		if payload[0] == 0 && payload[1] == 3 {
			loadAddr = binary.LittleEndian.Uint16(payload[14:16])
			continue
		}
		if payload[0] == 0xFF && loadAddr != 0 && blk == nil {
			blk = payload[1 : n-1]
		}
	}
	if blk == nil {
		t.Fatal("no CODE block")
	}
	prev := cliFlagsActive
	nf := cliFlags{}
	if prev != nil {
		nf = *prev
	}
	nf.noSound = true
	cliFlagsActive = &nf
	defer func() { cliFlagsActive = prev }()

	emu, err := newEmulator(roms.ModelPlus2)
	if err != nil {
		t.Fatal(err)
	}
	emu.paused.Store(false)
	for i := 0; i < 220; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	// Hook AFTER boot: catch every FLASH-bit attr write by demo code.
	hits := 0
	emu.mem.SetRAMWriteHook(func(bank int, addr uint16, val byte) {
		// hook bank = 16K RAM page index; attr region = bank-relative offs>=0x1800.
		isAttr := (bank == 5 || bank == 7) && addr >= 0x1800
		if val&0x80 != 0 && isAttr && hits < 60 {
			hits++
			sb := emu.mem.Read(0x8A15)
			t.Logf("CFWRITE page=%d off=%04X val=%02X pc=%04X scrbase=%02X AF=%04X BC=%04X DE=%04X HL=%04X",
				bank, addr, val, emu.cpu.PC, sb,
				uint16(emu.cpu.A)<<8|uint16(emu.cpu.F), emu.cpu.BC(), emu.cpu.DE(), emu.cpu.HL())
		}
	})
	defer emu.mem.SetRAMWriteHook(nil)
	for i, b := range blk {
		emu.mem.Write(uint16(loadAddr)+uint16(i), b)
	}
	emu.cpu.SP = 0xFF00
	emu.mem.Write(0xFEFF, 0x00)
	emu.mem.Write(0xFEFE, 0x00)
	emu.cpu.PC = loadAddr
	emu.cpu.IFF1, emu.cpu.IFF2 = false, false
	emu.cpu.IM = 1
	seen := map[string]int{}
	for i := 0; i < 200; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	// re-dump the row 6 strip attrs
	for _, n := range []int{5, 7} {
		p := emu.mem.RAM8KPage(2 * n)
		s := ""
		for c := 0; c < 32; c++ {
			v := p[(0x1800+6*32)+c]
			if v == 0x0F {
				s += "."
			} else if v == 0x17 {
				s += "R"
			} else {
				s += fmt.Sprintf("%02X", v)
			}
		}
		t.Logf("after200 b%d r6 %s", n, s)
	}
	_ = seen
}
