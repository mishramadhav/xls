package xls

import (
	"bytes"
	"encoding/binary"
	"math"
	"unicode/utf16"
)

// Helpers that build raw BIFF8 and CFB bytes for cases xlwt cannot produce.

func u16s(v ...uint16) []byte {
	var b []byte
	for _, x := range v {
		b = binary.LittleEndian.AppendUint16(b, x)
	}
	return b
}

func u32s(v ...uint32) []byte {
	var b []byte
	for _, x := range v {
		b = binary.LittleEndian.AppendUint32(b, x)
	}
	return b
}

func cat(parts ...[]byte) []byte { return bytes.Join(parts, nil) }

func rec(id uint16, body ...[]byte) []byte {
	b := cat(body...)
	return cat(u16s(id, uint16(len(b))), b)
}

func bofRec(vers, dt uint16) []byte {
	return rec(0x809, u16s(vers, dt, 0, 0), u32s(0, 0))
}

func utf16le(s string) []byte {
	var b []byte
	for _, u := range utf16.Encode([]rune(s)) {
		b = binary.LittleEndian.AppendUint16(b, u)
	}
	return b
}

func f64(f float64) []byte { return binary.LittleEndian.AppendUint64(nil, math.Float64bits(f)) }

func numberRec(r, c, xf uint16, f float64) []byte { return rec(0x203, u16s(r, c, xf), f64(f)) }
func rkRec(r, c, xf uint16, rk uint32) []byte     { return rec(0x27E, u16s(r, c, xf), u32s(rk)) }
func blankRec(r, c uint16) []byte                 { return rec(0x201, u16s(r, c, 0)) }
func sstIdxRec(r, c uint16, i uint32) []byte      { return rec(0xFD, u16s(r, c, 0), u32s(i)) }
func boolErrRec(r, c uint16, v, isErr byte) []byte {
	return rec(0x205, u16s(r, c, 0), []byte{v, isErr})
}
func labelRec(r, c uint16, s string) []byte {
	return rec(0x204, u16s(r, c, 0, uint16(len(s))), []byte{0}, []byte(s))
}

func mulRKRec(r, first uint16, rks ...uint32) []byte {
	var body []byte
	for _, rk := range rks {
		body = cat(body, u16s(0), u32s(rk))
	}
	return rec(0xBD, u16s(r, first), body, u16s(first+uint16(len(rks))-1))
}

func mulBlankRec(r, first, n uint16) []byte {
	return rec(0xBE, u16s(r, first), make([]byte, 2*n), u16s(first+n-1))
}

func formulaRec(r, c, xf uint16, result [8]byte) []byte {
	return rec(0x06, u16s(r, c, xf), result[:], u16s(0), u32s(0), []byte{0, 0})
}

func stringRec(s string) []byte { return rec(0x207, u16s(uint16(len(s))), []byte{0}, []byte(s)) }

func xfRec(fmtNo uint16) []byte { return rec(0xE0, u16s(0, fmtNo), make([]byte, 16)) }

func formatRec(idx uint16, s string) []byte {
	return rec(0x41E, u16s(idx, uint16(len(s))), []byte{0}, []byte(s))
}

// xlsFile returns a BIFF8 workbook stream with one sheet; globals are extra
// records placed before the BOUNDSHEET record.
func xlsFile(globals [][]byte, sheet ...[]byte) []byte {
	head := cat(bofRec(0x600, 5), cat(globals...))
	bs := func(pos uint32) []byte {
		return rec(0x85, u32s(pos), []byte{0, 0, 6, 0}, []byte("Sheet1"))
	}
	end := rec(0x0A)
	pos := uint32(len(head) + len(bs(0)) + len(end))
	return cat(head, bs(pos), end, bofRec(0x600, 0x10), cat(sheet...), rec(0x0A))
}

const (
	eoc  = 0xFFFFFFFE
	free = 0xFFFFFFFF
)

func dirEntry(name string, typ byte, child, start uint32, size uint64) []byte {
	e := make([]byte, 128)
	copy(e, utf16le(name))
	binary.LittleEndian.PutUint16(e[64:], uint16(len(name)*2+2))
	e[66] = typ
	binary.LittleEndian.PutUint32(e[68:], free)
	binary.LittleEndian.PutUint32(e[72:], free)
	binary.LittleEndian.PutUint32(e[76:], child)
	binary.LittleEndian.PutUint32(e[116:], start)
	binary.LittleEndian.PutUint64(e[120:], size)
	return e
}

// cfbFile wraps stream as the "Workbook" stream of a compound file with sector
// size ss (512: version 3, 4096: version 4). Streams under 4096 bytes use the
// mini stream; more than 109 FAT sectors use DIFAT sectors.
func cfbFile(stream []byte, ss int) []byte {
	epf := ss / 4
	var sectors [][]byte
	var fat []uint32
	chain := func(data []byte) (start uint32, n int) {
		if len(data) == 0 {
			return eoc, 0
		}
		n = (len(data) + ss - 1) / ss
		start = uint32(len(sectors))
		for i := 0; i < n; i++ {
			sec := make([]byte, ss)
			copy(sec, data[i*ss:])
			sectors = append(sectors, sec)
			next := start + uint32(i) + 1
			if i == n-1 {
				next = eoc
			}
			fat = append(fat, next)
		}
		return start, n
	}
	rootStart, miniStart, numMini, wbStart := uint32(eoc), uint32(eoc), 0, uint32(0)
	var rootSize uint64
	if len(stream) < 4096 {
		nm := (len(stream) + 63) / 64
		var mfat []byte
		for i := 0; i < nm; i++ {
			next := uint32(i + 1)
			if i == nm-1 {
				next = eoc
			}
			mfat = binary.LittleEndian.AppendUint32(mfat, next)
		}
		mini := make([]byte, nm*64)
		copy(mini, stream)
		rootStart, _ = chain(mini)
		rootSize = uint64(len(mini))
		miniStart, numMini = chain(mfat)
	} else {
		wbStart, _ = chain(stream)
	}
	dir := cat(dirEntry("Root Entry", 5, 1, rootStart, rootSize), dirEntry("Workbook", 2, free, wbStart, uint64(len(stream))))
	for len(dir)%ss != 0 {
		dir = cat(dir, dirEntry("", 0, free, 0, 0))
	}
	dirStart, dirN := chain(dir)

	nFAT, nDIFAT := 1, 0
	for {
		nDIFAT = 0
		if nFAT > 109 {
			nDIFAT = (nFAT - 109 + epf - 2) / (epf - 1)
		}
		if nFAT*epf >= len(fat)+nFAT+nDIFAT {
			break
		}
		nFAT++
	}
	fatIDs := make([]uint32, nFAT)
	for i := range fatIDs {
		fatIDs[i] = uint32(len(sectors) + i)
		fat = append(fat, 0xFFFFFFFD)
	}
	difatStart := uint32(eoc)
	if nDIFAT > 0 {
		difatStart = uint32(len(sectors) + nFAT)
	}
	for i := 0; i < nDIFAT; i++ {
		fat = append(fat, 0xFFFFFFFC)
	}
	for len(fat) < nFAT*epf {
		fat = append(fat, free)
	}
	for i := 0; i < nFAT; i++ {
		sec := make([]byte, ss)
		for j := 0; j < epf; j++ {
			binary.LittleEndian.PutUint32(sec[4*j:], fat[i*epf+j])
		}
		sectors = append(sectors, sec)
	}
	for d := 0; d < nDIFAT; d++ {
		sec := make([]byte, ss)
		for j := 0; j < epf-1; j++ {
			v := uint32(free)
			if k := 109 + d*(epf-1) + j; k < nFAT {
				v = fatIDs[k]
			}
			binary.LittleEndian.PutUint32(sec[4*j:], v)
		}
		next := uint32(eoc)
		if d < nDIFAT-1 {
			next = difatStart + uint32(d) + 1
		}
		binary.LittleEndian.PutUint32(sec[4*(epf-1):], next)
		sectors = append(sectors, sec)
	}

	major, shift, numDir := uint16(3), uint16(9), uint32(0)
	if ss == 4096 {
		major, shift, numDir = 4, 12, uint32(dirN)
	}
	h := make([]byte, ss)
	copy(h, []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1})
	copy(h[24:], u16s(0x3E, major, 0xFFFE, shift, 6))
	copy(h[40:], u32s(numDir, uint32(nFAT), dirStart, 0, 4096, miniStart, uint32(numMini), difatStart, uint32(nDIFAT)))
	for i := 0; i < 109; i++ {
		v := uint32(free)
		if i < nFAT {
			v = fatIDs[i]
		}
		binary.LittleEndian.PutUint32(h[76+4*i:], v)
	}
	return cat(h, cat(sectors...))
}
