package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

// TestSinescrollPTrace — watch p every instruction at the wrap-site.
func TestSinescrollPTrace(t *testing.T) {
	syms := map[string]uint16{}
	bs, _ := os.ReadFile("/root/nerve-workspace/demos/sinescroll/sinescroll.sym")
	for _, ln := range strings.Split(string(bs), "\n") {
		name, rest, ok := strings.Cut(strings.TrimSpace(ln), ":")
		if !ok {
			continue
		}
		rest = strings.TrimSpace(rest)
		if strings.HasPrefix(rest, "EQU 0x") {
			if v, e := strconv.ParseUint(strings.Fields(rest[6:])[0], 16, 16); e == nil {
				syms[name] = uint16(v)
			}
		}
	}
	raw, _ := os.ReadFile("/root/nerve-workspace/demos/sinescroll/sinescroll.tap")
	var code []byte
	var base uint16
	off := 0
	var pend uint16
	for off+2 <= len(raw) {
		n := int(binary.LittleEndian.Uint16(raw[off:]))
		off += 2
		p := raw[off : off+n]
		off += n
		if len(p) < 16 {
			continue
		}
		if p[0] == 0 && p[1] == 3 {
			pend = binary.LittleEndian.Uint16(p[14:16])
			continue
		}
		if p[0] == 0xFF && pend != 0 {
			base = pend
			code = p[1 : n-1]
			pend = 0
		}
	}
	nf := cliFlags{}
	if cliFlagsActive != nil {
		nf = *cliFlagsActive
	}
	nf.noSound = true
	sv := cliFlagsActive
	cliFlagsActive = &nf
	defer func() { cliFlagsActive = sv }()
	emu, err := newEmulator(roms.ModelPlus2)
	if err != nil {
		t.Fatal(err)
	}
	emu.paused.Store(false)
	for i := 0; i < 220; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	for i, v := range code {
		emu.mem.Write(base+uint16(i), v)
	}
	emu.cpu.SP = 0xFF00
	emu.cpu.PC = base
	wrapSite := syms["wpk"] // ld (p),hl inside proc
	fmt.Printf("p-write site $%04X\n", wrapSite)
	nFrame := 0
	loopTop := syms["loop"]
	wpkHits := 0
	procHits := 0
	for i := 0; i < 6000000; i++ {
		pc := emu.cpu.PC
		if pc == syms["proc"] {
			procHits++
		}
		emu.cpu.StepInstructionWithIRQ()
		if pc == wrapSite {
			wpkHits++
			hl := int(emu.cpu.HL()) // value about to be stored to p
			lvl := emu.mem.Read(syms["lvl"])
			pp := int(emu.mem.Read(syms["p"])) | int(emu.mem.Read(syms["p"]+1))<<8
			if lvl == 1 {
				fmt.Printf("WPK f%d pre=%d post=%d\n", nFrame, pp, hl)
			}
		}
		if pc == loopTop {
			nFrame++
			if nFrame > 4200 {
				break
			}
		}
	}
	fmt.Printf("wpk hits total=%d proc=%d\n", wpkHits, procHits)
	pp := int(emu.mem.Read(syms["p"])) | int(emu.mem.Read(syms["p"]+1))<<8
	fmt.Printf("end: frames=%d p=%d lvl%d\n", nFrame, pp, emu.mem.Read(syms["lvl"]))
}
