package xls

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
)

// WorkBook is an xls workbook.
type WorkBook struct {
	Is5ver   bool
	Type     uint16
	Codepage uint16
	Xfs      []st_xf_data
	Fonts    []Font
	Formats  map[uint16]*Format
	// sheets from the workbook
	sheets   []*WorkSheet
	Author   string
	data     []byte
	sst      []string
	dateMode uint16
}

// readStruct fills v from b and reports whether b was long enough.
func readStruct(b []byte, v any) bool {
	return len(b) >= binary.Size(v) && binary.Read(bytes.NewReader(b), binary.LittleEndian, v) == nil
}

// parseGlobals reads the workbook globals substream up to its EOF record.
func (w *WorkBook) parseGlobals() error {
	rr := &recReader{data: w.data}
	id, body, ok := rr.next()
	if !ok || id != recBOF || len(body) < 4 {
		return errors.New("xls: workbook stream does not start with a BOF record")
	}
	if le16(body, 2) != 0x0005 {
		return errors.New("xls: first substream is not the workbook globals")
	}
	w.Is5ver = le16(body, 0) != 0x600
	w.Type = le16(body, 2)
	for depth := 1; ; {
		id, body, ok := rr.next()
		if !ok {
			return errors.New("xls: workbook globals truncated before EOF record")
		}
		switch id {
		case recBOF:
			depth++
			continue
		case recEOF:
			if depth--; depth == 0 {
				return nil
			}
			continue
		}
		if depth > 1 {
			continue
		}
		switch id {
		case recFilePass:
			return errors.New("xls: encrypted workbook is not supported")
		case recCodepage:
			if len(body) >= 2 {
				w.Codepage = le16(body, 0)
			}
		case recDateMode:
			if len(body) >= 2 {
				w.dateMode = le16(body, 0)
			}
		case recSST:
			segs := [][]byte{body}
			for next, ok := rr.peekID(); ok && next == recContinue; next, ok = rr.peekID() {
				_, b, ok := rr.next()
				if !ok {
					return errors.New("xls: truncated CONTINUE record")
				}
				segs = append(segs, b)
			}
			sst, err := parseSST(segs)
			if err != nil {
				return err
			}
			w.sst = sst
		case recBoundSheet:
			w.addSheet(body)
		case recXF:
			w.addXf(body)
		case recFont:
			w.addFont(body)
		case recFormat:
			w.addFormat(body)
		}
	}
}

func (w *WorkBook) addSheet(b []byte) {
	if len(b) < 7 {
		return
	}
	if len(w.sheets) >= maxSheets {
		return
	}
	bs := &boundsheet{Filepos: le32(b, 0), Visible: b[4] & 3, Type: b[5], Name: b[6]}
	name := w.xlString(b[7:], int(b[6]))
	w.sheets = append(w.sheets, &WorkSheet{bs: bs, Name: name, wb: w, Visibility: TWorkSheetVisibility(bs.Visible)})
}

// addXf keeps a zero XF for a short record so later XF indexes stay aligned.
func (w *WorkBook) addXf(b []byte) {
	if w.Is5ver {
		xf := new(Xf5)
		readStruct(b, xf)
		w.Xfs = append(w.Xfs, xf)
		return
	}
	xf := new(Xf8)
	readStruct(b, xf)
	w.Xfs = append(w.Xfs, xf)
}

func (w *WorkBook) addFont(b []byte) {
	f := new(FontInfo)
	if !readStruct(b, f) {
		w.Fonts = append(w.Fonts, Font{Info: f})
		return
	}
	name := w.xlString(b[binary.Size(f):], int(f.NameB))
	w.Fonts = append(w.Fonts, Font{Info: f, Name: name})
}

func (w *WorkBook) addFormat(b []byte) {
	if len(b) < 4 {
		return
	}
	f := new(Format)
	f.Head.Index = le16(b, 0)
	if w.Is5ver {
		f.Head.Size = uint16(b[2])
		f.str = w.xlString(b[3:], int(f.Head.Size))
	} else {
		f.Head.Size = le16(b, 2)
		f.str = w.xlString(b[4:], int(f.Head.Size))
	}
	w.Formats[f.Head.Index] = f
}

// prepareSheet parses the sheet substream, keeping any cells read before an error.
func (w *WorkBook) prepareSheet(sheet *WorkSheet) error {
	if int64(sheet.bs.Filepos) >= int64(len(w.data)) {
		return fmt.Errorf("xls: sheet %q offset outside the workbook stream", sheet.Name)
	}
	return sheet.parse(w.data[sheet.bs.Filepos:])
}

// GetSheet returns one sheet by its number, or nil if there is no such sheet.
// Parse problems do not panic; see WorkSheet.Err.
func (w *WorkBook) GetSheet(num int) *WorkSheet {
	if num < 0 || num >= len(w.sheets) {
		return nil
	}
	s := w.sheets[num]
	if !s.parsed {
		s.err = w.safePrepare(s)
	}
	return s
}

func (w *WorkBook) safePrepare(s *WorkSheet) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("xls: malformed sheet %q: %v", s.Name, r)
		}
		s.parsed = true
	}()
	return w.prepareSheet(s)
}

// NumSheets returns the number of sheets.
func (w *WorkBook) NumSheets() int {
	return len(w.sheets)
}

// ReadAllCells reads the cells of all sheets, at most max rows in total.
// Each row holds the cell values from column 0; rows without cells are nil.
// It needs much memory for large files.
func (w *WorkBook) ReadAllCells(max int) (res [][]string) {
	res = make([][]string, 0)
	for i := range w.sheets {
		if len(res) >= max {
			break
		}
		s := w.GetSheet(i)
		if len(s.rows) == 0 {
			continue
		}
		n := min(int(s.MaxRow)+1, max-len(res))
		temp := make([][]string, n)
		for k := 0; k < n; k++ {
			if row := s.rows[uint16(k)]; row != nil && len(row.cols) > 0 {
				temp[k] = row.values()
			}
		}
		res = append(res, temp...)
	}
	return res
}
