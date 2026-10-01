package sources

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// zstdDecodeFile decompresses a file's frames through the zstd CLI, keeping
// what it can when one frame is bad (#4294).
//
// A log still being appended, or one whose writer died mid-frame, ends in a
// torn frame. zstd says "premature end" and has written every complete frame
// before it to stdout, so that output is kept. Any other failure is a frame
// that is complete and does not decode, and zstd stops there with good frames
// possibly after it; the file is then split into frames and each decoded on its
// own. Every frame lost either way counts as one unusable line in the ingest
// diagnostics, which names the file instead of the store. Only a file nothing
// decodes from is an error.
func zstdDecodeFile(path, harness string, raw []byte) ([]byte, error) {
	out, stderr, err := zstdRun(raw)
	if err == nil {
		return out, nil
	}
	if strings.Contains(stderr, "premature end") && len(out) > 0 {
		diagMalformedLine(path)
		return out, nil
	}
	frames, rest := zstdSplitFrames(raw)
	var kept []byte
	for _, f := range frames {
		dec, _, ferr := zstdRun(f)
		if ferr != nil {
			diagMalformedLine(path)
			continue
		}
		kept = append(kept, dec...)
	}
	if len(rest) > 0 {
		// A torn last frame: what zstd decodes of it before the cut is kept,
		// as on the fast path.
		dec, _, _ := zstdRun(rest)
		kept = append(kept, dec...)
		diagMalformedLine(path)
	}
	if len(kept) == 0 {
		return nil, fmt.Errorf("%s: zstd -d %s: %w: %s", harness, filepath.Base(path), err, stderr)
	}
	return kept, nil
}

func zstdRun(in []byte) (out []byte, stderr string, err error) {
	cmd := exec.Command("zstd", "-d", "-c", "-q")
	cmd.Stdin = bytes.NewReader(in)
	var o, e bytes.Buffer
	cmd.Stdout = &o
	cmd.Stderr = &e
	err = cmd.Run()
	return o.Bytes(), strings.TrimSpace(e.String()), err
}

// zstdSplitFrames cuts a stream at its frame boundaries by reading the frame
// and block headers (RFC 8878 §3.1), without decoding anything. rest is what
// follows the last frame that is whole: a torn frame, or bytes that are not a
// frame at all.
func zstdSplitFrames(raw []byte) (frames [][]byte, rest []byte) {
	for len(raw) > 0 {
		n := zstdFrameLen(raw)
		if n <= 0 {
			return frames, raw
		}
		frames = append(frames, raw[:n])
		raw = raw[n:]
	}
	return frames, nil
}

// zstdFrameLen is the length of the frame raw starts with, or 0 when raw does
// not hold a whole one.
func zstdFrameLen(raw []byte) int {
	if len(raw) < 8 {
		return 0
	}
	magic := binary.LittleEndian.Uint32(raw)
	if magic&0xFFFFFFF0 == 0x184D2A50 {
		// A skippable frame: magic, a 4-byte size, then that many bytes.
		n := 8 + int(binary.LittleEndian.Uint32(raw[4:]))
		if n > len(raw) {
			return 0
		}
		return n
	}
	if magic != 0xFD2FB528 {
		return 0
	}
	fhd := raw[4]
	single := fhd&0x20 != 0
	pos := 5
	if !single {
		pos++ // window descriptor
	}
	pos += []int{0, 1, 2, 4}[fhd&0x03] // dictionary id
	switch fhd >> 6 {                  // frame content size
	case 0:
		if single {
			pos++
		}
	case 1:
		pos += 2
	case 2:
		pos += 4
	case 3:
		pos += 8
	}
	for {
		if pos+3 > len(raw) {
			return 0
		}
		h := uint32(raw[pos]) | uint32(raw[pos+1])<<8 | uint32(raw[pos+2])<<16
		pos += 3
		last := h&1 != 0
		size := int(h >> 3)
		switch (h >> 1) & 3 {
		case 1: // RLE: one byte, repeated
			size = 1
		case 3: // reserved: not a frame
			return 0
		}
		pos += size
		if last {
			break
		}
	}
	if fhd&0x04 != 0 {
		pos += 4 // content checksum
	}
	if pos > len(raw) {
		return 0
	}
	return pos
}
