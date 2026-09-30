package main

// Throwaway v4b probe: byte-compare the scroller band (y184..191) against
// mkfont.py goldens (bandref.txt) on the +2 core. Delete once v4b is locked.

import (
	"fmt"
	"encoding/binary"
	"encoding/hex"
	"strconv"
	"os"
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

	syms := map[string]uint16{}
	if bs, err := os.ReadFile("/root/nerve-workspace/demos/snow/snow.sym"); err != nil {
		t.Fatalf("read sym: %v", err)
	} else {
		for _, ln := range strings.Split(string(bs), "\n") {
			name, rest, ok := strings.Cut(strings.TrimSpace(ln), ":")
			if !ok {
				continue
			}
			if i := strings.Index(rest, "EQU "); i >= 0 {
				f := strings.Fields(strings.TrimSpace(rest[i+4:]))
				if len(f) > 0 && len(f[0]) > 2 && f[0][:2] == "0x" {
					f[0] = f[0][2:]
				}
				if len(f) > 0 {
					if v, err := strconv.ParseUint(f[0], 16, 16); err == nil {
						syms[name] = uint16(v)
					}
				}
				continue
			}
			// sjasmplus label-only form: "name:" = current location counter.
			// Value may carry a page/bank suffix like "$811F:8000" — take
			// the field up to the colon.
			t := strings.TrimSpace(rest)
			if j := strings.IndexByte(t, ':'); j >= 0 {
				t = t[:j]
			}
			if strings.HasPrefix(t, "$") {
				t = t[1:]
			}
			if v, err := strconv.ParseUint(t, 16, 16); err == nil {
				syms[name] = uint16(v)
			}
		}
	}
	adr := func(n string) uint16 {
		v, ok := syms[n]
		if !ok {
			t.Fatalf("sym %s missing", n)
		}
		return v
	}
	srollA, svalA := adr("sroll"), adr("sval")
	hmapA := adr("hmap")
	txtpgA, txtoffA, ptmrA, fldsA := adr("txtpg"), adr("txtoff"), adr("ptmr"), adr("flds")
		fcntA, modeA := adr("fcnt"), adr("mode")
	haltA := uint16(0x8119) // loop-head HALT (verified PC at park, fcnt 73)
	_ = modeA
	_ = svalA
	t.Logf("syms: sroll=%04X sval=%04X txtpg=%04X txtoff=%04X ptmr=%04X flds=%04X", srollA, svalA, txtpgA, txtoffA, ptmrA, fldsA)

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
			t.Fatalf("bad golden line: %q", ln[:40])
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

	readBand := func() []byte {
		b := make([]byte, 256)
		pg := emu.mem.RAM8KPage(10)
		for r := 0; r < 8; r++ {
			copy(b[r*32:r*32+32], pg[0x10E0+r*0x100:])
		}
		return b
	}

	dumpf, _ := os.Create("/tmp/v4b.dump")
	defer dumpf.Close()
	dump := dumpf
	matched, failed := 0, 0
	for i := 0; i < 6000 && len(gold) > 0; i++ {
		// PARK first: spin frames (guarded) until the CPU rests at the
		// loop-head HALT. Only there is (sroll, band) a coherent pair —
		// reads anywhere else land mid-paint or mid-quantum.
		parked := false
		for k := 0; k < 42000; k++ {
			if emu.cpu.PC == haltA && emu.cpu.Halted {
				parked = true
				break
			}
			emu.cpu.StepInstruction()
		}
		if !parked {
			t.Fatalf("CPU never parks at HALT %04X by frame %d (pc=%04X)", haltA, i, emu.cpu.PC)
		}
		p := uint16(emu.mem.Read(srollA)) | uint16(emu.mem.Read(srollA+1))<<8
		emu.mem.Write(ptmrA, 250) // pin storm timer: no drain/wipe (which
		// resets sroll) — let sroll run the full 4256 px period incl. seam.
		if os.Getenv("SNOW_NOFLAKE") != "" {
			for f := 0; f < 36; f++ {
				emu.mem.Write(fldsA+uint16(f*4+1), 0xFF) // kill flakes: y=$FF
			}
		}
		got := readBand()
		if os.Getenv("SNOW_DUMP") != "" {
			maxy, mh, nz := 0, 0, 0
			for f := 0; f < 36; f++ {
				if y := int(emu.mem.Read(fldsA + uint16(f*4+1))); y != 0xFF && y > maxy {
					maxy = y
				}
			}
			for o := 0; o < 256; o++ {
				if h := int(emu.mem.Read(hmapA + uint16(o))); h > mh {
					mh = h
				}
			}
			for _, v := range got {
				if v != 0 {
					nz++
				}
			}
			fmt.Fprintf(dump, "%d %s F=%d NZ=%d\n", p, hex.EncodeToString(got),
				emu.mem.Read(fcntA), nz)
			if p == 7 || p == 100 || p == 3000 || p == 3935 || p == 3999 || p == 4000 || p == 4254 || p == 4255 {
				wb := uint16(emu.mem.Read(txtpgA))<<8 | uint16(emu.mem.Read(txtoffA))
				win := make([]byte, 33)
				for j := range win {
					win[j] = emu.mem.Read(wb + uint16(j))
				}
				fmt.Fprintf(dump, "#WIN p=%d wb=%04X win=%s\n", p, wb, hex.EncodeToString(win))
				fk := make([]byte, 48)
				for j := range fk {
					fk[j] = emu.mem.Read(fldsA + uint16(j))
				}
				fmt.Fprintf(dump, "#FLK p=%d %s\n", p, hex.EncodeToString(fk))
				ra, _ := os.ReadFile("/root/nerve-workspace/demos/snow/bandref.txt")
				for _, ln := range strings.Split(string(ra), "\n") {
					fs := strings.Fields(ln)
					if len(fs) == 2 && fs[0] == fmt.Sprintf("%d", p) {
						gb, _ := hex.DecodeString(fs[1])
						for i := 0; i < 256; i++ {
							if got[i] != gb[i] {
								fmt.Fprintf(dump, "#DIF p=%d i=%d r%d c%d got=%02X gold=%02X\n",
									p, i, i/32, i%32, got[i], gb[i])
							}
						}
					}
				}
			}
			if maxy >= 160 || mh >= 16 {
				fmt.Fprintf(dump, "#STAT p=%d maxy=%d maxh=%d\n", p, maxy, mh)
			}
		}
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
		// advance one frame from the HALT so the next park sees new content
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	t.Logf("matched=%d failed=%d unmatched=%d", matched, failed, len(gold))
	if failed > 0 {
		t.Fatalf("%d golden frames mismatched", failed)
	}
	if matched < len(gold)+failed || matched == 0 {
		t.Fatalf("only %d goldens reached", matched)
	}
}
