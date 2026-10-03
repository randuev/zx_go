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

// TestSinescrollMidFrame — the honesty check: snapshot the screen DURING a
// frame, not just at the committed park. If mid-frame shows sparse
// fragments (like Seva's real-+2 photo), the engine cost overruns a frame.
func TestSinescrollMidFrame(t *testing.T) {
	syms := map[string]uint16{}
	bs, err := os.ReadFile("/root/nerve-workspace/demos/sinescroll/sinescroll.sym")
	if err != nil {
		t.Fatalf("read sym: %v", err)
	}
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
	sym := func(n string) uint16 { return syms[n] }
	raw, err := os.ReadFile("/root/nerve-workspace/demos/sinescroll/sinescroll.tap")
	if err != nil {
		t.Fatalf("read tap: %v", err)
	}
	var blocks [][2]any
	off := 0
	var pending uint16
	for off+2 <= len(raw) {
		n := int(binary.LittleEndian.Uint16(raw[off:]))
		off += 2
		if off+n > len(raw) {
			break
		}
		p := raw[off : off+n]
		off += n
		if len(p) < 16 {
			continue
		}
		if p[0] == 0 && p[1] == 3 {
			pending = binary.LittleEndian.Uint16(p[14:16])
			continue
		}
		if p[0] == 0xFF && pending != 0 {
			blocks = append(blocks, [2]any{pending, p[1 : n-1]})
			pending = 0
		}
	}
	base, code := blocks[0][0].(uint16), blocks[0][1].([]byte)
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
		t.Fatalf("boot: %v", err)
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
	pVal := func() int { return int(emu.mem.Read(sym("p"))) | int(emu.mem.Read(sym("p")+1))<<8 }
	lvl := func() byte { return emu.mem.Read(sym("lvl")) }
	screen := func() []byte { return emu.mem.RAM8KPage(10)[:6144] }
	ink := func() int {
		n := 0
		for _, b := range screen() {
			if b != 0 {
				n++
			}
		}
		return n
	}
	walk := sym("walk")
	proc := sym("proc")
	// lead-in
	for i := 0; i < 2000 && (pVal() <= 300 || lvl() != 0); i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	// Now step frame-by-frame; log walk entries and ink at several
	// sub-frame offsets, plus border port writes (port $FE low bits).
	procHits := 0
	for frames := 0; frames < 8; frames++ {
		start := emu.cpu.Tstates()
		var marks []string
		step := 0
		for emu.cpu.Tstates()-start < 70908 && step < 400000 {
			emu.cpu.StepInstructionWithIRQ()
			step++
			pc := emu.cpu.PC
			dt := emu.cpu.Tstates() - start
			if pc == walk {
				marks = append(marks, fmt.Sprintf("walk@%d", dt))
			}
			if pc == proc && dt > 100 {
				marks = append(marks, fmt.Sprintf("proc@%d", dt))
				procHits++
			}
			if dt%20000 < 4 && len(marks) < 60 && dt > 0 && (len(marks)==0 || marks[len(marks)-1] != fmt.Sprintf("ink@%d", dt/20000*20000)) {
				marks = append(marks, fmt.Sprintf("ink=%d@%d", ink(), dt))
			}
		}
		fmt.Printf("FRAME %d ink=%d marks=%v\n", frames, ink(), marks)
	}
	fmt.Printf("MIDFRAME procHits=%d\n", procHits)
	// ground-truth PNG at frame END (committed) and mid
	capScreen2 := func(path string) {
		f := func(p string) {
			raw := make([]byte, 256*192*3)
			page := emu.mem.RAM8KPage(10)
			for y := 0; y < 192; y++ {
				for x := 0; x < 256; x++ {
					o := ((y & 7) << 8) + ((y & 0x38) << 2) + ((y & 0xC0) << 5) + (x >> 3)
					i := (y*256 + x) * 3
					if page[o]&(0x80>>(x&7)) != 0 {
						raw[i], raw[i+1], raw[i+2] = 0xFF, 0xFF, 0xFF
					}
				}
			}
			if err := writePNG(p, 256, 192, raw); err != nil {
				t.Fatal(err)
			}
		}
		f(path)
	}
	capScreen2("/tmp/mid_end.png")
}
