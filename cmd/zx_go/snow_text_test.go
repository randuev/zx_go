package main

// Text-as-text gate (2026-10-01). Proves buildscroll reproduces the oracle
// image: after boot the $9000-$9AFF RAM region must be byte-identical to
// scroll.ref.bin (font transposed from the resident ROM + glyph-index stream
// built from the BUILDTEXT literal in snow.zasm). Checked mid-boot AND after
// hundreds of scroll frames (nothing may clobber the pages during play).
// Delete if the scroller ever moves away from runtime-built pages.

import (
	"os"
	"strings"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestSnowTextBuild(t *testing.T) {
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
	// ref must exist and be full-size (else the test would silently pass)
	ref, err := os.ReadFile("/root/nerve-workspace/demos/snow/scroll.ref.bin")
	if err != nil {
		t.Fatal(err)
	}
	if len(ref) != 2816 {
		t.Fatalf("ref wrong size %d", len(ref))
	}
	var code []byte
	off := 0
	for off+2 <= len(raw) {
		n := int(raw[off]) | int(raw[off+1])<<8
		off += 2
		p := raw[off : off+n]
		off += n
		if len(p) < 16 {
			continue
		}
		if p[0] == 0xFF && n > 2000 {
			code = p[1 : n-1]
		}
	}
	if code == nil {
		t.Fatal("no CODE block")
	}
	if len(code) != 0x1800 {
		t.Fatalf("block must span $8000..$97FF (6144 B): %d", len(code))
	}
	if !strings.Contains(string(code), "In 1979,") {
		t.Fatal("BUILDTEXT literal not inside code block — text-as-text lost")
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
	// Poison ONLY the runtime-built slice pages $9800..$9AFF (the static
	// font $9000-$97FF is tap data — poking poison there would erase it).
	for a := 0; a < 768; a++ {
		emu.mem.Write(0x9800+uint16(a), 0xA5)
	}
	emu.cpu.SP = 0xFF00
	emu.mem.Write(0xFEFF, 0x00)
	emu.mem.Write(0xFEFE, 0x00)
	emu.cpu.PC = 0x8000
	emu.cpu.IFF1, emu.cpu.IFF2 = false, false
	emu.cpu.IM = 1

	// neutralise kfull (ghost-key stub at its entry, found via CALL + JPZ)
	kfullA := uint16(0)
	for i := 0; i+5 < len(code); i++ {
		if code[i] == 0xCD && code[i+3] == 0xCA {
			kfullA = uint16(code[i+1]) | uint16(code[i+2])<<8
			break
		}
	}
	if kfullA == 0 {
		t.Fatal("kfull call site not found")
	}
	emu.mem.Write(kfullA, 0xAF)
	emu.mem.Write(kfullA+1, 0xC9)

	{
		var romb []byte
		for a := 0; a < 8; a++ {
			romb = append(romb, emu.mem.Read(uint16(0x3D00+a)))
			romb = append(romb, emu.mem.Read(uint16(0x3D08+a)))
		}
		t.Logf("post-boot mem@3D00=%x @3D08(!)=%x", romb[:8], romb[8:])
		var e0 []byte
		for a := 0; a < 6; a++ {
			e0 = append(e0, emu.mem.Read(uint16(0x0000+a)))
		}
		t.Logf("mem@0000=%x", e0)
	}

	check := func(atFrame int) int {
		bad := 0
		regions := map[int]int{}
		for a := 0; a < 2816; a++ {
			if got := emu.mem.Read(0x9000 + uint16(a)); got != ref[a] {
				bad++
				regions[a/256]++
			}
		}
		t.Logf("f%d mismatching 256B regions: %v", atFrame, regions)
		return bad
	}

	for f := 0; f < 8; f++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	bad := check(8)
	if bad == 0 {
		// positive control: poison must be gone (buildscroll ran)
		_ = emu
	}
	for f := 8; f < 400; f++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	bad2 := check(400)
	t.Logf("buildscroll mismatch: early=%d after-scroll=%d", bad, bad2)
	if bad > 0 {
		t.Fatalf("buildscroll image wrong early: %d bytes", bad)
	}
	if bad2 > 0 {
		t.Fatalf("scroll frames clobbered text pages: %d bytes", bad2)
	}
}
