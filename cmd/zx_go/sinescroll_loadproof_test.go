package main

import (
	"fmt"
	"os"
	"testing"
)

// TestSinescrollLoadProof — what bytes actually land at $9980..$99C0?
// Compare tap-side (as parseTap hands it to the test loader) vs RAM side.
func TestSinescrollLoadProof(t *testing.T) {
	raw, err := os.ReadFile("/root/nerve-workspace/demos/sinescroll/sinescroll.tap")
	if err != nil {
		t.Fatal(err)
	}
	blocks := parseTap(raw)
	fmt.Printf("blocks=%d\n", len(blocks))
	base, code := blocks[0][0].(uint16), blocks[0][1].([]byte)
	fmt.Printf("base=%04X len(code)=%d (want %d)\n", base, len(code), 0xB4C0-0x8000)
	o := 0x9980 - 0x8000
	fmt.Printf("tap-side  @9980: % X\n", code[o:o+16])
	fmt.Printf("tap-side  @9A20: % X\n", code[o+160:o+168])
	fmt.Printf("tap-side @B4B0..: tail len=%d % X\n", len(code)-o-160, code[len(code)-8:])
	// top of block: does code even contain SHEET8 (0xB4C0-8 offset)?
	fmt.Printf("code byte[0]=%02X byte[0x34BF]=%02X\n", code[0], code[len(code)-1])
}
