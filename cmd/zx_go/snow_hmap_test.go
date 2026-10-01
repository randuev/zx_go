package main

// Throwaway: dump hmap + flake Ys to see why the band shows ink at low Y.

import (
	"encoding/binary"
	"fmt"
	"os"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestSnowHmapDump(t *testing.T) {
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
		t.Fatalf("read tap: %v", err)
	}
	var code []byte
	var pending uint16
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
			pending = binary.LittleEndian.Uint16(payload[14:16])
			continue
		}
		if payload[0] == 0xFF && pending != 0 {
			code = payload[1 : n-1]
		}
	}

	emu, err := newEmulator(roms.ModelPlus2)
	if err != nil {
		t.Fatalf("emu: %v", err)
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

	hmap := uint16(0x8A00) // will locate by scan below
	flds := uint16(0x8A80)
	fldc := uint16(0x8AC0)
	scrub := uint16(0x8AC8)

	// locate hmap: the routine writes ix+=2 (screen) and hl+=1 (hmap);
	// instead just scan $8A00-$8B00 — print full window every frame.
	for frame := 0; frame < 1200; frame++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
		if frame%100 != 99 {
			continue
		}
		c := emu.mem.Read(fldc)
		if c > 36 {
			c = 36
		}
		maxY := 0
		for i := 0; i < int(c); i++ {
			if y := int(emu.mem.Read(flds + uint16(i*4 + 1))); y > maxY {
				maxY = y
			}
		}
		hb := make([]byte, 256)
		mh := 0
		for i := 0; i < 256; i++ {
			v := emu.mem.Read(hmap + uint16(i))
			hb[i] = v
			if int(v) > mh {
				mh = int(v)
			}
		}
		sb := emu.mem.Read(scrub)
		fmt.Printf("f%d active=%d maxY=%d maxHmap=%d scrub=%d\nhmap %s\n", frame+1, c, maxY, mh, sb, fmt.Sprintf("%x", hb))
		_ = hmap
	}
}
