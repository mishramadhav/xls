// Package ole2 reads streams from a Compound File Binary container ([MS-CFB]).
//
// Derived from github.com/extrame/ole2 (Apache-2.0); see NOTICE.
package ole2

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf16"
)

const (
	maxRegSect = 0xFFFFFFFA
	endOfChain = 0xFFFFFFFE
	noStream   = 0xFFFFFFFF

	miniSectorSize = 64
	miniCutoff     = 4096
	dirEntrySize   = 128

	// MaxFileSize bounds memory: the whole container is read into memory.
	MaxFileSize = 256 << 20
)

const (
	typeStream = 2
	typeRoot   = 5
)

var signature = []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}

type entry struct {
	name               string
	typ                byte
	left, right, child uint32
	start              uint32
	size               uint64
}

// Ole is an opened compound file.
type Ole struct {
	data       []byte
	ss         int
	v4         bool
	fat        []uint32
	miniFAT    []uint32
	miniStream []byte
	miniLoaded bool
	entries    []entry
	hdr        []byte
}

// Open reads the whole container from r and validates its header, FAT and directory.
func Open(r io.Reader) (*Ole, error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxFileSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxFileSize {
		return nil, errors.New("ole2: file too large")
	}
	o := &Ole{data: data}
	if err := o.readHeader(); err != nil {
		return nil, err
	}
	if err := o.readFAT(); err != nil {
		return nil, err
	}
	if err := o.readDir(); err != nil {
		return nil, err
	}
	return o, nil
}

func le32(b []byte, off int) uint32 { return binary.LittleEndian.Uint32(b[off:]) }
func le16(b []byte, off int) uint16 { return binary.LittleEndian.Uint16(b[off:]) }

func (o *Ole) readHeader() error {
	h := o.data
	if len(h) < 512 || string(h[:8]) != string(signature) {
		return errors.New("ole2: not a compound file")
	}
	o.hdr = h[:512]
	major, shift := le16(h, 26), le16(h, 30)
	if le16(h, 28) != 0xFFFE {
		return errors.New("ole2: bad byte order")
	}
	switch {
	case major == 3 && shift == 9:
		o.ss = 512
	case major == 4 && shift == 12:
		o.ss, o.v4 = 4096, true
	default:
		return fmt.Errorf("ole2: unsupported version %d / sector shift %d", major, shift)
	}
	if le16(h, 32) != 6 {
		return errors.New("ole2: bad mini sector shift")
	}
	if le32(h, 56) != miniCutoff {
		return errors.New("ole2: bad mini stream cutoff")
	}
	return nil
}

func (o *Ole) sector(sid uint32) ([]byte, error) {
	off := (int64(sid) + 1) * int64(o.ss)
	if sid > maxRegSect || off >= int64(len(o.data)) {
		return nil, fmt.Errorf("ole2: sector %d out of range", sid)
	}
	return o.data[off:min(off+int64(o.ss), int64(len(o.data)))], nil
}

func (o *Ole) fullSector(sid uint32) ([]byte, error) {
	s, err := o.sector(sid)
	if err == nil && len(s) != o.ss {
		err = fmt.Errorf("ole2: sector %d truncated", sid)
	}
	return s, err
}

func (o *Ole) readFAT() error {
	numFAT, firstDIFAT, numDIFAT := le32(o.hdr, 44), le32(o.hdr, 68), le32(o.hdr, 72)
	maxSectors := uint32(len(o.data)/o.ss + 1)
	if numFAT > maxSectors || numDIFAT > maxSectors {
		return errors.New("ole2: FAT/DIFAT count exceeds file size")
	}
	var fatSecs []uint32
	for i := 0; i < 109 && uint32(len(fatSecs)) < numFAT; i++ {
		fatSecs = append(fatSecs, le32(o.hdr, 76+4*i))
	}
	next := firstDIFAT
	for i := uint32(0); i < numDIFAT && uint32(len(fatSecs)) < numFAT; i++ {
		s, err := o.fullSector(next)
		if err != nil {
			return err
		}
		n := o.ss/4 - 1
		for j := 0; j < n && uint32(len(fatSecs)) < numFAT; j++ {
			fatSecs = append(fatSecs, le32(s, 4*j))
		}
		next = le32(s, 4*n)
	}
	if uint32(len(fatSecs)) != numFAT {
		return errors.New("ole2: DIFAT shorter than FAT sector count")
	}
	for _, sid := range fatSecs {
		s, err := o.fullSector(sid)
		if err != nil {
			return err
		}
		for j := 0; j < o.ss; j += 4 {
			o.fat = append(o.fat, le32(s, j))
		}
	}
	return nil
}

// chain follows a sector chain, rejecting links to free sectors, to sectors
// beyond the n that exist, and cycles. It bounds the chain by the real data.
func chain(start uint32, table []uint32, n int) ([]uint32, error) {
	n = min(n, len(table))
	var out []uint32
	for sid := start; sid != endOfChain; {
		if sid > maxRegSect || int(sid) >= n {
			return nil, fmt.Errorf("ole2: bad sector id %#x in chain", sid)
		}
		if len(out) >= n {
			return nil, errors.New("ole2: sector chain loops")
		}
		out = append(out, sid)
		sid = table[sid]
	}
	return out, nil
}

// readRegular reads a FAT-backed stream; size < 0 means the whole chain.
func (o *Ole) readRegular(start uint32, size int64) ([]byte, error) {
	sids, err := chain(start, o.fat, (len(o.data)+o.ss-1)/o.ss-1)
	if err != nil {
		return nil, err
	}
	avail := int64(len(sids)) * int64(o.ss)
	if size < 0 {
		size = avail
	}
	if size > avail {
		return nil, errors.New("ole2: stream size exceeds its sector chain")
	}
	out := make([]byte, 0, min(size, int64(len(o.data))))
	for _, sid := range sids {
		if int64(len(out)) >= size {
			break
		}
		s, err := o.sector(sid)
		if err != nil {
			return nil, err
		}
		out = append(out, s[:min(int64(len(s)), size-int64(len(out)))]...)
	}
	if int64(len(out)) != size {
		return nil, errors.New("ole2: stream truncated")
	}
	return out, nil
}

func (o *Ole) loadMini() error {
	if o.miniLoaded {
		return nil
	}
	root := o.entries[0]
	ms, err := o.readRegular(root.start, int64(min(root.size, MaxFileSize)))
	if err != nil {
		return err
	}
	raw, err := o.readRegular(le32(o.hdr, 60), -1)
	if err != nil {
		return err
	}
	for j := 0; j+4 <= len(raw); j += 4 {
		o.miniFAT = append(o.miniFAT, le32(raw, j))
	}
	o.miniStream, o.miniLoaded = ms, true
	return nil
}

func (o *Ole) readMini(start uint32, size int64) ([]byte, error) {
	if err := o.loadMini(); err != nil {
		return nil, err
	}
	sids, err := chain(start, o.miniFAT, (len(o.miniStream)+miniSectorSize-1)/miniSectorSize)
	if err != nil {
		return nil, err
	}
	if size > int64(len(sids))*miniSectorSize {
		return nil, errors.New("ole2: mini stream size exceeds its sector chain")
	}
	out := make([]byte, 0, min(size, int64(len(o.miniStream))))
	for _, sid := range sids {
		if int64(len(out)) >= size {
			break
		}
		off := int64(sid) * miniSectorSize
		if off >= int64(len(o.miniStream)) {
			return nil, errors.New("ole2: mini sector outside mini stream")
		}
		s := o.miniStream[off:min(off+miniSectorSize, int64(len(o.miniStream)))]
		out = append(out, s[:min(int64(len(s)), size-int64(len(out)))]...)
	}
	if int64(len(out)) != size {
		return nil, errors.New("ole2: mini stream truncated")
	}
	return out, nil
}

func (o *Ole) readDir() error {
	raw, err := o.readRegular(le32(o.hdr, 48), -1)
	if err != nil {
		return err
	}
	for off := 0; off+dirEntrySize <= len(raw); off += dirEntrySize {
		o.entries = append(o.entries, o.parseEntry(raw[off:off+dirEntrySize]))
	}
	if len(o.entries) == 0 || o.entries[0].typ != typeRoot {
		return errors.New("ole2: missing root entry")
	}
	return nil
}

func (o *Ole) parseEntry(b []byte) entry {
	e := entry{typ: b[66], left: le32(b, 68), right: le32(b, 72), child: le32(b, 76), start: le32(b, 116), size: uint64(le32(b, 120))}
	if o.v4 {
		e.size |= uint64(le32(b, 124)) << 32
	}
	if n := int(le16(b, 64)); n >= 2 && n <= 64 && n%2 == 0 {
		u := make([]uint16, n/2-1)
		for i := range u {
			u[i] = le16(b, 2*i)
		}
		e.name = string(utf16.Decode(u))
	} else if n != 0 {
		e.typ = 0 // invalid name length: treat as unallocated
	}
	if e.typ != 1 && e.typ != typeStream && e.typ != typeRoot {
		e.typ = 0
	}
	return e
}

// Stream returns the content of the first of names found among the root's direct children.
func (o *Ole) Stream(names ...string) ([]byte, error) {
	for _, want := range names {
		if e := o.find(want); e != nil {
			return o.open(e)
		}
	}
	return nil, fmt.Errorf("ole2: stream %q not found", names)
}

// find walks the directory tree of the root storage.
func (o *Ole) find(name string) *entry {
	stack := []uint32{o.entries[0].child}
	for visited := 0; len(stack) > 0 && visited <= len(o.entries); visited++ {
		id := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if id == noStream || int(id) >= len(o.entries) {
			continue
		}
		e := &o.entries[id]
		if e.typ == typeStream && strings.EqualFold(e.name, name) {
			return e
		}
		stack = append(stack, e.left, e.right)
	}
	return nil
}

func (o *Ole) open(e *entry) ([]byte, error) {
	if e.size == 0 {
		return nil, nil
	}
	if e.size > MaxFileSize {
		return nil, errors.New("ole2: stream too large")
	}
	if e.size < miniCutoff {
		return o.readMini(e.start, int64(e.size))
	}
	return o.readRegular(e.start, int64(e.size))
}
