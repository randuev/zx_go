package main

// Throwaway probe: byte-compare the scroller band (y184..191) against
// mkfont.py goldens (bandref.txt) on the +2 core. Anchors are derived at
// runtime (fresh syms when parseable, raw-byte scans otherwise) because
// stale hardcoded hex silently mis-hooked the whole v4b era once already.
// Delete once the scroller budget is locked.

import (
	"encoding/binary"
	"encoding/hex"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestSnowV4bGoldenBand(t *testing.T) {
	prev := cliFlagsActive
	nf := cliFlags{}
	if prev != nil {
		nf = *prev
	}
	nf.noSound = true
	cliFlagsActive = &nf
	defer func() { cliFlagsActive = prev }()

	tapPath := "/root/nerve-workspace/demos/snow/snow.tap"
	if tp := os.Getenv("SNOW_TAP"); tp != "" {
		tapPath = tp
	}
	raw, err := os.ReadFile(tapPath)
	if err != nil {
		t.Fatalf("read tap: %v", err)
	}
	type tapBlk struct {
		addr uint16
		data []byte
	}
	var blocks []tapBlk
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
			blocks = append(blocks, tapBlk{pending, payload[1 : n-1]})
			pending = 0
		}
	}
	if len(blocks) < 1 {
		t.Fatalf("no CODE block")
	}
	code := blocks[0].data
	base := blocks[0].addr

	syms := map[string]uint16{}
	if bs, err := os.ReadFile("/root/nerve-workspace/demos/snow/snow.sym"); err == nil {
		for _, ln := range strings.Split(string(bs), "\n") {
			name, rest, ok := strings.Cut(strings.TrimSpace(ln), ":")
			if !ok {
				continue
			}
			t := strings.TrimSpace(rest)
			if i := strings.Index(t, "EQU "); i >= 0 {
				t = strings.TrimSpace(t[i+4:])
			}
			if j := strings.IndexByte(t, ';'); j >= 0 {
				t = t[:j]
			}
			if k := strings.Index(t, "EQU"); k >= 0 {
				t = t[k+3:]
			}
			t = strings.TrimSpace(t)
			t = strings.TrimPrefix(t, "$")
			t = strings.TrimPrefix(t, "0x")
			if v, err := strconv.ParseUint(t, 16, 16); err == nil {
				syms[name] = uint16(v)
			}
		}
	}
	// E3 (magenta-before-sscroll) = ld a,$E3 ; out ($FE),a ; call sscroll
	e3A := uint16(0)
	for i := 0; i+7 < len(code); i++ {
		if code[i] == 0x3E && code[i+1] == 0xE3 && code[i+2] == 0xD3 && code[i+3] == 0xFE && code[i+4] == 0xCD {
			e3A = base + uint16(i)
			break
		}
	}
	if e3A == 0 {
		t.Fatalf("E3 site not found in code")
	}
		// Loop head via fresh syms (sjasmplus DOES emit bare labels as EQU
	// now — the old "jr -> 0x76" backward scan died when back-branches
	// became jp loop). HALT = first 0x76 within 16 bytes of loop head
	// (current layout: ld a,$E0/out/halt).
	loopA, haltA := uint16(0), uint16(0)
	if bs, err := os.ReadFile("/root/nerve-workspace/demos/snow/snow.sym"); err == nil {
		for _, ln := range strings.Split(string(bs), "\n") {
			n, rest, ok := strings.Cut(strings.TrimSpace(ln), ":")
			if !ok || n != "loop" {
				continue
			}
			t2 := strings.TrimSpace(rest)
			if k := strings.Index(t2, "EQU"); k >= 0 {
				t2 = t2[k+3:]
			}
			t2 = strings.TrimSpace(t2)
			t2 = strings.TrimPrefix(t2, "$")
			t2 = strings.TrimPrefix(t2, "0x")
			if v, err := strconv.ParseUint(t2, 16, 16); err == nil {
				loopA = uint16(v)
			}
			break
		}
	}
	if loopA == 0 {
		t.Fatalf("loop sym not in snow.sym")
	}
	for i := int(loopA) - int(base); i < int(loopA)-int(base)+16 && i < len(code); i++ {
		if code[i] == 0x76 {
			haltA = base + uint16(i)
			break
		}
	}
	if haltA == 0 {
		t.Fatalf("HALT not found within 16B of loop head")
	}
t.Logf("anchors: e3=%04X loop=%04X halt=%04X", e3A, loopA, haltA)

	srollA := syms["sroll"]
	ptmrA := syms["ptmr"]
	if srollA == 0 || ptmrA == 0 {
		t.Fatalf("sroll/ptmr syms missing")
	}

	gold := map[uint16][]byte{}
	gs, err := os.ReadFile("/root/nerve-workspace/demos/snow/bandref.txt")
	if err != nil {
		t.Fatalf("read bandref: %v", err)
	}
	for _, ln := range strings.Split(string(gs), "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" || strings.HasPrefix(ln, "#") {
			continue
		}
		f := strings.Fields(ln)
		if len(f) != 2 || len(f[1]) != 512 {
			t.Fatalf("bad golden line: %q", ln[:min(len(ln), 40)])
		}
		bs, err := hex.DecodeString(f[1])
		if err != nil {
			t.Fatalf("bad golden hex: %v", err)
		}
		var p uint64
		for _, ch := range f[0] {
			p = p*10 + uint64(ch-'0')
		}
		gold[uint16(p)] = bs
	}
	t.Logf("goldens loaded: %d frames", len(gold))

	emu, err := newEmulator(roms.ModelPlus2)
	if err != nil {
		t.Fatalf("newEmulator(+2): %v", err)
	}
	emu.paused.Store(false)
	for i := 0; i < 220; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	for _, b := range blocks {
		for i, v := range b.data {
			emu.mem.Write(uint16(b.addr)+uint16(i), v)
		}
	}
	emu.cpu.SP = 0xFF00
	emu.mem.Write(0xFEFF, 0x00)
	emu.mem.Write(0xFEFE, 0x00)
	emu.cpu.PC = 0x8000
	emu.cpu.IFF1, emu.cpu.IFF2 = false, false
	emu.cpu.IM = 1
	// Neutralise the headless ghost-key quit flake (documented class). Stub
	// kfull := xor a ; ret (Z=yes -> "no key") found by its call pattern.
	kfullA := uint16(0)
	for i := 0; i+5 < len(code); i++ {
		if code[i] == 0xCD && code[i+3] == 0xCA {
			kfullA = base + uint16(code[i+1]) | uint16(code[i+2])<<8
			break
		}
	}
	if kfullA == 0 {
		t.Fatalf("kfull call site not found")
	}
	t.Logf("kfull target=%04X", kfullA)
	emu.mem.Write(kfullA, 0xAF)
	emu.mem.Write(kfullA+1, 0xC9)

	readBand := func() []byte {
		b := make([]byte, 256)
		pg := emu.mem.RAM8KPage(10)
		for r := 0; r < 8; r++ {
			copy(b[r*32:r*32+32], pg[0x10E0+r*0x100:])
		}
		return b
	}

	matched, failed := 0, 0
	for i := 0; i < 6000 && len(gold) > 0; i++ {
		parked := false
		for k := 0; k < 42000; k++ {
			if emu.cpu.PC == haltA+1 && emu.cpu.Halted {
				parked = true
				break
			}
			emu.cpu.StepInstruction()
		}
		if !parked {
			t.Fatalf("CPU never parks at HALT+1 %04X by frame %d (pc=%04X)", haltA+1, i, emu.cpu.PC)
		}
		p := uint16(emu.mem.Read(srollA)) | uint16(emu.mem.Read(srollA+1))<<8
		emu.mem.Write(ptmrA, 250) // pin storm timer: no drain/wipe resets
		got := readBand()
		if g, ok := gold[p]; ok {
			if string(got) == string(g) {
				matched++
				t.Logf("p=%d MATCH", p)
			} else {
				failed++
				t.Errorf("p=%d MISMATCH", p)
				for r := 0; r < 8; r++ {
					d0, d1 := r*32, r*32+32
					if string(got[d0:d1]) != string(g[d0:d1]) {
						t.Errorf("  row r%d want %s", r, hex.EncodeToString(g[d0:d1]))
						t.Errorf("  row r%d got  %s", r, hex.EncodeToString(got[d0:d1]))
					}
				}
			}
			delete(gold, p)
		}
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	t.Logf("matched=%d failed=%d unmatched=%d", matched, failed, len(gold))
	if failed > 0 {
		t.Fatalf("%d golden frames mismatched", failed)
	}
	if matched == 0 {
		t.Fatalf("no goldens reached")
	}
}
