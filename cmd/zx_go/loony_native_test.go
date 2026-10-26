package main

// loony_native_test.go — LOONYNATIVE=1: THE definitive repro+catch.
// Boots loony.tap NATIVELY through the tape trap with in-process ENTER
// (same tape, same boot path as Seva's +2). Once CODE lands at $8000, a
// WriteObserver armed at that instant flags every write into the code page
// ($8000-$87FF). live telnet proved $80C0/$82BF/$84C0/$86C0 bytes shift
// +1 right with $00 inserts on native boots — freezing title()/paintbuf —
// while injection boots never bite. This test prints the writer PC.
import (
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestLoonyNative(t *testing.T) {
	if os.Getenv("LOONYNATIVE") == "" {
		t.Skip("set LOONYNATIVE=1")
	}
	mapRaw, _ := os.ReadFile("/tmp/loony.sym")
	syms := map[string]uint16{}
	re := regexp.MustCompile(`(\w+): (?:EQU|DEF[SE]) 0x0000([0-9A-Fa-f]{4})`)
	for _, m := range re.FindAllStringSubmatch(string(mapRaw), -1) {
		var v uint64
		fmt.Sscanf(m[2], "%x", &v)
		syms[m[1]] = uint16(v)
	}
	prev := cliFlagsActive
	nf := cliFlags{}
	nf.noSound = true
	nf.headless = true
	cliFlagsActive = &nf
	defer func() { cliFlagsActive = prev }()
	emu, err := newEmulator(roms.ModelPlus2)
	if err != nil {
		t.Fatal(err)
	}
	emu.paused.Store(false)
	if err := loadTapeFile(emu, "/root/nerve-workspace/demos/loony/loony.tap"); err != nil {
		t.Fatal(err)
	}
	installTapeTrap(emu)
	// ring buffer of PCs for trail
	const ringN = 4096
	ring := make([]uint16, ringN)
	ringI := 0
	var hits [][5]uint16 // addr,val,pc,hl,de
	armed := false
	firstSP := uint16(0)
	var firstStack [10]byte
	spAt := map[uint16]uint16{}
	emu.mem.WriteObserver = func(addr uint16, val byte, _ uint16) {
		if !armed {
			return
		}
		if addr >= 0x8000 && addr <= 0x87FF {
			if len(hits) == 0 {
				firstSP = emu.cpu.SP
				for i := 0; i < 10; i++ {
					firstStack[i] = emu.mem.Read(emu.cpu.SP+uint16(i))
				}
			}
			if len(hits) < 64 {
				hits = append(hits, [5]uint16{addr, uint16(val), emu.cpu.PC, emu.cpu.HL(), emu.cpu.DE()})
				if pc, ok := spAt[emu.cpu.PC]; !ok {
					spAt[emu.cpu.PC] = emu.cpu.SP
					_ = pc
				}
			}
		}
	}
	defer func() { emu.mem.WriteObserver = nil }()

	landed := false
	for f := 0; f < 3000 && !landed; f++ {
		// hold ENTER from frame ~40 until the loader arms
		if f == 40 {
			emu.kbd.PressMatrixKey(6, 0x01, true)
		}
		if f == 80 {
			emu.kbd.PressMatrixKey(6, 0x01, false)
		}
		if f == 160 {
			emu.kbd.PressMatrixKey(6, 0x01, true)
		}
		if f == 200 {
			emu.kbd.PressMatrixKey(6, 0x01, false)
		}
		// free-run the frame; record PC each instruction via TrapCheck-ish
		runOneFrameHeadless(emu, roms.ModelPlus2)
		if !armed && emu.mem.Read(0x8000) == 0xF3 {
			armed = true
			t.Logf("ARMED at frame %d", f)
		}
		if armed {
			ring[ringI&65535] = emu.cpu.PC
			ringI++
			if len(hits) > 0 {
				break
			}
			if f > 400 {
				// CLEAN FIX PROVEN: no writes into the code page at all.
				t.Logf("armed %d frames, ZERO code-page writes — shift_row D=0 fix holds natively", f-168)
				t0 := emu.mem.Read(syms["t"])
				tx0 := emu.mem.Read(syms["tx"])
				for i := 0; i < 300; i++ {
					runOneFrameHeadless(emu, roms.ModelPlus2)
				}
				t1 := emu.mem.Read(syms["t"])
				tx1 := emu.mem.Read(syms["tx"])
				t.Logf("liveness: t $%02X->$%02X tx $%02X->$%02X pc=$%04X", t0, t1, tx0, tx1, emu.cpu.PC)
				if len(hits) > 0 {
					t.Errorf("code page written during liveness phase: %+v", hits[:min(8, len(hits))])
				}
				if t1 == t0 {
					t.Errorf("t frozen — loop dead")
				}
				if tx0 == tx1 {
					t.Errorf("tx frozen — scroll dead")
				}
				return
			}
		}
	}
	if !armed {
		t.Fatal("native land never happened")
	}
	if len(hits) == 0 {
		t.Log("no writes into code page — native boot clean in harness")
		// prove liveness: the loop must keep advancing t and tx past land
		t0 := emu.mem.Read(syms["t"])
		tx0 := emu.mem.Read(syms["tx"])
		for i := 0; i < 300; i++ {
			runOneFrameHeadless(emu, roms.ModelPlus2)
		}
		t1 := emu.mem.Read(syms["t"])
		tx1 := emu.mem.Read(syms["tx"])
		t.Logf("liveness: t $%02X->$%02X tx $%02X->$%02X pc=$%04X", t0, t1, tx0, tx1, emu.cpu.PC)
		if t1 == t0 {
			t.Errorf("t frozen after fix — loop dead")
		}
		if tx0 == tx1 {
			t.Errorf("tx frozen after fix — scroll dead")
		}
		return
	}
	fmt.Printf("FIRST CODE-PAGE WRITES (native boot):\n")
	for i, h := range hits[:min(24, len(hits))] {
		fmt.Printf("  #%d addr=$%04X val=$%02X pc=$%04X\n", i, h[0], byte(h[1]), h[2])
	}
	fmt.Printf("first-hit SP=$%04X stack=% X\n", firstSP, firstStack)
	fmt.Printf("return addrs from stack:  AF=%02X%02X BC=%04X DE=%04X HL=%04X IX=%04X IY=%04X SP=%04X\n",
		emu.cpu.A, emu.cpu.F, emu.cpu.BC(), emu.cpu.DE(), emu.cpu.HL(), emu.cpu.IX, emu.cpu.IY, emu.cpu.SP)
	fmt.Printf("scrbase=%02X curX=%02X curY=%02X swb=%02X sht=%02X bx0=%02X t=%02X tx=%02X\n",
		emu.mem.Read(syms["scrbase"]), emu.mem.Read(syms["curX"]), emu.mem.Read(syms["curY"]),
		emu.mem.Read(syms["swb"]), emu.mem.Read(syms["sht"]), emu.mem.Read(syms["bx0"]),
		emu.mem.Read(syms["t"]), emu.mem.Read(syms["tx"]))
	fmt.Printf("mmu: $8000=%02X $C000=%02X $4000=%02X\n", emu.mem.GetMMU(4), emu.mem.GetMMU(6), emu.mem.GetMMU(2))
	// stack frame return addresses
	for i := 0; i < 10; i += 2 {
		fmt.Printf("  [SP+%d]=$%04X\n", i, uint16(firstStack[i+1])<<8|uint16(firstStack[i]))
	}
	fmt.Printf("shift_row bytes $8258-$8272: ")
	for a := uint16(0x8258); a <= 0x8272; a++ {
		fmt.Printf("%02X ", emu.mem.Read(a))
	}
	fmt.Println()
	// PC trail
	n := ringI
	if n > 400 {
		n = 400
	}
	fmt.Print("PC TRAIL:")
	for i := ringI - n; i < ringI; i++ {
		if (i-(ringI-n))%50 == 0 {
			fmt.Print("\n ")
		}
		fmt.Printf(" %04X", ring[i&(ringN-1)])
	}
	fmt.Println()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
