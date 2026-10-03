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

// TestSinescrollPHigh — WHO writes p's high byte? Watch $8E01 every step.
func TestSinescrollPHigh(t *testing.T) {
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
	// snapshot globals block
	var prev [32]byte
	for k := range prev {
		prev[k] = emu.mem.Read(0x8E00 + uint16(k))
	}
	loopTop := syms["loop"]
	nLoop := 0
	for i := 0; i < 2000000; i++ {
		pc := emu.cpu.PC
		emu.cpu.StepInstructionWithIRQ()
		var cur [32]byte
		changed := false
		for k := range cur {
			cur[k] = emu.mem.Read(0x8E00 + uint16(k))
			if cur[k] != prev[k] {
				changed = true
			}
		}
		if changed && (prev[0]!=cur[0] || prev[1]!=cur[1]) && emu.mem.Read(syms["lvl"])==1 {
			pv := int(prev[0]) | int(prev[1])<<8
			cv := int(cur[0]) | int(cur[1])<<8
			fmt.Printf("i=%d pc=$%04X p %d->%d DE=%04X HL=%04X\n", i, pc, pv, cv, emu.cpu.DE(), emu.cpu.HL())
		} else if changed {
			var d []string
			for k := range cur {
				if cur[k] != prev[k] {
					d = append(d, fmt.Sprintf("$8E%02X:%02X>%02X", k, prev[k], cur[k]))
				}
			}
			_ = d
		}
		prev = cur
		if pc == loopTop {
			nLoop++
			if nLoop > 180 {
				break
			}
		}
	}
}
