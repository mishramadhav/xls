package xls

import (
	"errors"
	"unicode/utf16"
)

var errSST = errors.New("xls: truncated or malformed SST")

// sstCursor walks the SST record and its CONTINUE records as one byte stream
// that remembers where each record ends: a string whose characters cross a
// record end restarts with a new fHighByte flag byte, other data does not.
type sstCursor struct {
	segs [][]byte
	si   int
	off  int
}

func (c *sstCursor) cur() []byte {
	if c.si < len(c.segs) {
		return c.segs[c.si]
	}
	return nil
}

// advance moves to the next non-empty record; false when none is left.
func (c *sstCursor) advance() bool {
	for c.si++; c.si < len(c.segs); c.si++ {
		if len(c.segs[c.si]) > 0 {
			c.off = 0
			return true
		}
	}
	return false
}

// take returns n contiguous bytes that must not cross a record end.
func (c *sstCursor) take(n int) ([]byte, bool) {
	for c.off >= len(c.cur()) {
		if !c.advance() {
			return nil, false
		}
	}
	if c.off+n > len(c.cur()) {
		return nil, false
	}
	b := c.cur()[c.off : c.off+n]
	c.off += n
	return b, true
}

func (c *sstCursor) skip(n int) bool {
	for n > 0 {
		if c.off >= len(c.cur()) {
			if !c.advance() {
				return false
			}
		}
		k := min(n, len(c.cur())-c.off)
		c.off += k
		n -= k
	}
	return true
}

// parseSST decodes the SST record body followed by its CONTINUE bodies.
func parseSST(segs [][]byte) ([]string, error) {
	if len(segs) == 0 || len(segs[0]) < 8 {
		return nil, errSST
	}
	total := 0
	for _, s := range segs {
		total += len(s)
	}
	unique := int(le32(segs[0], 4))
	if unique > total/3 { // every string takes at least 3 bytes
		return nil, errSST
	}
	c := &sstCursor{segs: segs, off: 8}
	out := make([]string, 0, unique)
	for len(out) < unique {
		hdr, ok := c.take(3)
		if !ok {
			return nil, errSST
		}
		cch, flags := int(le16(hdr, 0)), hdr[2]
		runs, ext := 0, 0
		if flags&8 != 0 {
			b, ok := c.take(2)
			if !ok {
				return nil, errSST
			}
			runs = int(le16(b, 0))
		}
		if flags&4 != 0 {
			b, ok := c.take(4)
			if !ok {
				return nil, errSST
			}
			ext = int(le32(b, 0))
		}
		var u []uint16
		high := flags&1 != 0
		for len(u) < cch {
			if c.off >= len(c.cur()) { // character data continues in the next record, after a new flag byte
				if !c.advance() {
					return nil, errSST
				}
				high = c.cur()[0]&1 != 0
				c.off = 1
				continue
			}
			seg, w := c.cur(), 1
			if high {
				w = 2
			}
			n := min((len(seg)-c.off)/w, cch-len(u))
			if n == 0 { // stray byte: a UTF-16 unit split across records
				return nil, errSST
			}
			for i := 0; i < n; i++ {
				if high {
					u = append(u, le16(seg, c.off+2*i))
				} else {
					u = append(u, uint16(seg[c.off+i]))
				}
			}
			c.off += n * w
		}
		if !c.skip(runs*4 + ext) {
			return nil, errSST
		}
		out = append(out, string(utf16.Decode(u)))
	}
	return out, nil
}
