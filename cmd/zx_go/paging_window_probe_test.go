package main

import (
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

// Which PHYSICAL bytes does each CPU window actually touch?
// TestCruxayPlus2Run's census says bank5 text persists even though
// paintbuf clears $C000 with slot=0x0D. Map every window byte to its
// physical 8K page byte to settle classic-128 routing once.
func TestPagingWindowMap(t *testing.T) {
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
	erase := func() {
		for p := 0; p < 16; p++ {
			h := emu.mem.RAM8KPage(p)
			for i := range h {
				h[i] = 0
			}
		}
	}
	// windows: addr, page-offset where its byte lands if window base page is P
	addrOff := []struct {
		name string
		addr uint16
		off  uint16
	}{
		{"$4000", 0x4000, 0x0000},
		{"$4840", 0x4840, 0x0840},
		{"$6000", 0x6000, 0x0000},
		{"$6840", 0x6840, 0x0840},
		{"$C000", 0xC000, 0x0000},
		{"$C840", 0xC840, 0x0840},
	}
	val := byte(0xA0)
	for _, bits := range []byte{0x05, 0x0D, 0x07, 0x0F} {
		emu.mem.PageMemory(bits)
		for _, w := range addrOff {
			erase()
			v := val | byte(w.addr>>8&0x0F)
			emu.mem.Write(w.addr, v)
			for p := 0; p < 16; p++ {
				h := emu.mem.RAM8KPage(p)
				if h[w.off] == v {
					t.Logf("pg=%02X write%s(v=%02X) -> page[%02d]+%04X", bits, w.name, v, p, w.off)
				}
			}
		}
		val += 0x10
	}
	t.Log("done")
}
