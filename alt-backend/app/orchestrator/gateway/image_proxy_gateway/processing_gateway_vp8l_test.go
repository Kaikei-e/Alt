package image_proxy_gateway

import (
	"context"
	"encoding/binary"
	"runtime"
	"testing"
	"time"
)

// TestProcessingGateway_ProcessImage_VP8LHuffmanGroupsStayBounded is the
// GO-2026-6222 / CVE-2026-46603 regression: a tiny lossless WebP can name a
// Huffman group index of 65535. golang.org/x/image before v0.45.0 allocated
// that many tree groups (~100MiB+) from a few dozen input bytes. The proxy
// must reject or decode without unbounded allocation. The payload is built
// in-process so the suite never checks in a large fixture.
func TestProcessingGateway_ProcessImage_VP8LHuffmanGroupsStayBounded(t *testing.T) {
	payload := craftedVP8LWebPWithHugeHuffmanIndex()
	if len(payload) > 256 {
		t.Fatalf("crafted payload must stay tiny, got %d bytes", len(payload))
	}

	var ms runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&ms)
	start := ms.TotalAlloc

	gw := NewProcessingGateway()
	done := make(chan error, 1)
	go func() {
		_, err := gw.ProcessImage(context.Background(), payload, "image/webp", 600, 80)
		done <- err
	}()

	select {
	case <-done:
		// Success or a decode error are both acceptable; hanging or exploding
		// the heap is not.
	case <-time.After(5 * time.Second):
		t.Fatal("VP8L decode did not finish; possible unbounded Huffman-group work")
	}

	runtime.ReadMemStats(&ms)
	grew := ms.TotalAlloc - start
	const maxAlloc = 32 << 20
	if grew > maxAlloc {
		t.Fatalf("decoded %d-byte WebP allocated %d bytes (cap %d); GO-2026-6222 unbounded Huffman groups",
			len(payload), grew, maxAlloc)
	}
}

// craftedVP8LWebPWithHugeHuffmanIndex is a 1×1 lossless WebP whose meta-image
// pixel names Huffman group 0xffff. Pre-v0.45 decoders then make([]hGroup, 65536).
func craftedVP8LWebPWithHugeHuffmanIndex() []byte {
	w := &lsbWriter{}
	w.write(0x2f, 8) // VP8L signature
	w.write(0, 14)   // width-1 = 0 → 1px
	w.write(0, 14)   // height-1 = 0 → 1px
	w.write(0, 1)    // hasAlpha
	w.write(0, 3)    // version
	w.write(0, 1)    // no transforms
	w.write(0, 1)    // no color cache (image)
	w.write(1, 1)    // meta Huffman image present
	w.write(0, 3)    // hBits-2 = 0 → 2; nTiles(1,2)=1
	w.write(0, 1)    // no color cache (meta)
	// One Huffman group for the 1×1 meta-image: 5 simple 1-symbol trees.
	// Green/red = 255 so the group index is 0xffff; blue/alpha/distance = 0.
	writeSimpleSymbol(w, 255)
	writeSimpleSymbol(w, 255)
	writeSimpleSymbol(w, 0)
	writeSimpleSymbol(w, 0)
	writeSimpleSymbol(w, 0)
	// nSymbols==1 means code length 0: the meta pixel needs no extra bits.

	return wrapWebPLossless(w.flush())
}

func writeSimpleSymbol(w *lsbWriter, symbol uint32) {
	w.write(1, 1) // use simple tree
	w.write(0, 1) // nSymbols-1 = 0 → one symbol
	if symbol > 1 {
		w.write(1, 1) // 8-bit symbol field
		w.write(symbol, 8)
		return
	}
	w.write(0, 1) // 1-bit symbol field
	w.write(symbol, 1)
}

func wrapWebPLossless(vp8l []byte) []byte {
	pad := len(vp8l) % 2
	riffSize := uint32(4 + 8 + len(vp8l) + pad)
	out := make([]byte, 0, 12+8+len(vp8l)+pad)
	out = append(out, 'R', 'I', 'F', 'F')
	out = binary.LittleEndian.AppendUint32(out, riffSize)
	out = append(out, 'W', 'E', 'B', 'P', 'V', 'P', '8', 'L')
	out = binary.LittleEndian.AppendUint32(out, uint32(len(vp8l)))
	out = append(out, vp8l...)
	if pad == 1 {
		out = append(out, 0)
	}
	return out
}

type lsbWriter struct {
	bits uint32
	n    uint32
	buf  []byte
}

func (w *lsbWriter) write(val, n uint32) {
	w.bits |= (val & (1<<n - 1)) << w.n
	w.n += n
	for w.n >= 8 {
		w.buf = append(w.buf, byte(w.bits))
		w.bits >>= 8
		w.n -= 8
	}
}

func (w *lsbWriter) flush() []byte {
	if w.n > 0 {
		w.buf = append(w.buf, byte(w.bits))
	}
	return w.buf
}
