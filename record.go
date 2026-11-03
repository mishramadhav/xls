package xls

import (
	"encoding/binary"
	"unicode/utf16"

	"golang.org/x/text/encoding/charmap"
)

// maxRecordSize is the largest record body [MS-XLS] 2.1.4 allows.
const maxRecordSize = 8224

const (
	recBOF        = 0x809
	recEOF        = 0x0A
	recContinue   = 0x3C
	recCodepage   = 0x42
	recFilePass   = 0x2F
	recDateMode   = 0x22
	recBoundSheet = 0x85
	recSST        = 0xFC
	recXF         = 0xE0
	recFont       = 0x31
	recFormat     = 0x41E
	recWindow2    = 0x23E
	recRow        = 0x208
	recNumber     = 0x203
	recRK         = 0x27E
	recMulRK      = 0xBD
	recMulBlank   = 0xBE
	recBlank      = 0x201
	recBoolErr    = 0x205
	recLabel      = 0x204
	recLabelSST   = 0xFD
	recFormula    = 0x06
	recString     = 0x207
	recShrFmla    = 0x4BC
	recArray      = 0x221
	recTable      = 0x236
	recHLink      = 0x1B8
)

func le16(b []byte, off int) uint16 { return binary.LittleEndian.Uint16(b[off:]) }
func le32(b []byte, off int) uint32 { return binary.LittleEndian.Uint32(b[off:]) }

type recReader struct {
	data []byte
	pos  int
}

// next returns the next record; ok is false at the end of data or on a
// truncated or oversized record.
func (r *recReader) next() (id uint16, body []byte, ok bool) {
	if r.pos+4 > len(r.data) {
		return 0, nil, false
	}
	id = le16(r.data, r.pos)
	size := int(le16(r.data, r.pos+2))
	if size > maxRecordSize || r.pos+4+size > len(r.data) {
		return 0, nil, false
	}
	body = r.data[r.pos+4 : r.pos+4+size]
	r.pos += 4 + size
	return id, body, true
}

func (r *recReader) peekID() (uint16, bool) {
	if r.pos+4 > len(r.data) {
		return 0, false
	}
	return le16(r.data, r.pos), true
}

// xlString decodes the character data of an XLUnicodeString whose character
// count was already read (BIFF8: flags byte, optional rich-text and phonetic
// headers, characters, then the skipped run and phonetic data). It returns the
// string; short input yields what is available.
func (w *WorkBook) xlString(b []byte, cch int) string {
	if w.Is5ver {
		return w.decodeANSI(b[:min(cch, len(b))])
	}
	if len(b) == 0 {
		return ""
	}
	flags := b[0]
	p := 1
	if flags&8 != 0 {
		if len(b) < p+2 {
			return ""
		}
		p += 2
	}
	if flags&4 != 0 {
		if len(b) < p+4 {
			return ""
		}
		p += 4
	}
	width := 1 + int(flags&1)
	n := min(cch, (len(b)-p)/width)
	var s string
	if width == 2 {
		u := make([]uint16, n)
		for i := range u {
			u[i] = le16(b, p+2*i)
		}
		s = string(utf16.Decode(u))
	} else {
		r := make([]rune, n)
		for i := range r {
			r[i] = rune(b[p+i])
		}
		s = string(r)
	}
	return s
}

var codepages = map[uint16]*charmap.Charmap{
	1250: charmap.Windows1250, 1251: charmap.Windows1251, 1252: charmap.Windows1252,
	1253: charmap.Windows1253, 1254: charmap.Windows1254, 1255: charmap.Windows1255,
	1256: charmap.Windows1256, 1257: charmap.Windows1257, 1258: charmap.Windows1258,
}

// decodeANSI decodes BIFF5 byte strings with the workbook CODEPAGE.
func (w *WorkBook) decodeANSI(b []byte) string {
	cm := codepages[w.Codepage]
	if cm == nil {
		cm = charmap.Windows1252
	}
	out, err := cm.NewDecoder().Bytes(b)
	if err != nil {
		return string(b)
	}
	return string(out)
}
