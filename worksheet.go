package xls

import (
	"errors"
	"math"
	"unicode/utf16"
)

type TWorkSheetVisibility byte

const (
	WorkSheetVisible    TWorkSheetVisibility = 0
	WorkSheetHidden     TWorkSheetVisibility = 1
	WorkSheetVeryHidden TWorkSheetVisibility = 2
)

// maxSheets bounds BOUNDSHEET records: each sheet is parsed again on access.
const maxSheets = 1024

// maxCells bounds the cells parsed over all sheets of a workbook (one full sheet).
const maxCells = 256 * 65536

// parseBudgetFactor and minParseBudget bound the record bytes read over all sheet
// parses of a workbook: factor times the workbook stream, at least minParseBudget.
// Sheets that start inside one another would otherwise scan the same bytes again.
const (
	parseBudgetFactor = 4
	minParseBudget    = 1 << 20
)

// maxCol is the last column BIFF8 allows.
const maxCol = 255

type boundsheet struct {
	Filepos uint32
	Visible byte
	Type    byte
	Name    byte
}

// WorkSheet in one WorkBook
type WorkSheet struct {
	bs         *boundsheet
	wb         *WorkBook
	Name       string
	Selected   bool
	Visibility TWorkSheetVisibility
	rows       map[uint16]*Row
	// MaxRow is the highest row number of the sheet (the count minus one).
	MaxRow      uint16
	HyperLinks  []*HyperLink
	parsed      bool
	err         error
	rightToLeft bool
}

// Err reports why the sheet could not be read completely, if so. Cells read
// before the problem are still available.
func (w *WorkSheet) Err() error { return w.err }

// Row returns row i, or nil if the sheet has no cell or ROW record for it.
func (w *WorkSheet) Row(i int) *Row {
	if i < 0 || i > 0xFFFF {
		return nil
	}
	return w.rows[uint16(i)]
}

// parse reads the sheet substream that starts at the BOF record in data.
func (w *WorkSheet) parse(data []byte) error {
	w.rows = make(map[uint16]*Row)
	rr := &recReader{data: data}
	id, body, ok := rr.next()
	if !ok || id != recBOF || len(body) < 4 {
		return errors.New("xls: sheet does not start with a BOF record")
	}
	var formula *FormulaCol
	for depth := 1; ; {
		id, body, ok := rr.next()
		if !ok {
			return errors.New("xls: sheet truncated before EOF record")
		}
		if w.wb.parseBytes += len(body) + 4; w.wb.parseBytes > max(parseBudgetFactor*len(w.wb.data), minParseBudget) {
			return errors.New("xls: workbook exceeds the parse budget")
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
		if w.wb.cells > maxCells {
			return errors.New("xls: workbook exceeds the cell limit")
		}
		// A string result follows its FORMULA, possibly after array/shared formula records.
		if id != recString && id != recShrFmla && id != recArray && id != recTable {
			formula = nil
		}
		switch id {
		case recWindow2:
			if len(body) >= 2 {
				opts := le16(body, 0)
				w.rightToLeft = opts&0x40 != 0
				w.Selected = opts&0x400 != 0
			}
		case recRow:
			r := new(rowInfo)
			if readStruct(body, r) {
				r.Fcell, r.Lcell = min(r.Fcell, maxCol+1), min(r.Lcell, maxCol+1)
				w.addRow(r)
			}
		case recMulRK:
			w.addMulRK(body)
		case recMulBlank:
			w.addMulBlank(body)
		case recNumber:
			if len(body) >= 14 {
				w.addCell(&NumberCol{Col: cellPos(body), Index: le16(body, 4), Float: float64frombits(body[6:14])})
			}
		case recRK:
			if len(body) >= 10 {
				w.addCell(&RkCol{Col: cellPos(body), Xfrk: XfRk{Index: le16(body, 4), Rk: RK(le32(body, 6))}})
			}
		case recLabelSST:
			if len(body) >= 10 {
				w.addCell(&LabelsstCol{Col: cellPos(body), Xf: le16(body, 4), Sst: le32(body, 6)})
			}
		case recLabel:
			if len(body) >= 8 {
				str := w.wb.xlString(body[8:], int(le16(body, 6)))
				w.addCell(&labelCol{BlankCol: BlankCol{Col: cellPos(body), Xf: le16(body, 4)}, Str: str})
			}
		case recBlank:
			if len(body) >= 6 {
				w.addCell(&BlankCol{Col: cellPos(body), Xf: le16(body, 4)})
			}
		case recBoolErr:
			if len(body) >= 8 {
				w.addCell(&BoolErrCol{Col: cellPos(body), Xf: le16(body, 4), Val: body[6], IsErr: body[7] != 0})
			}
		case recFormula:
			if len(body) >= 20 {
				c := &FormulaCol{}
				readStruct(body[:20], &c.Header)
				c.Bts = body[20:]
				w.addCell(c)
				formula = c
			}
		case recString:
			if formula != nil && len(body) >= 2 {
				formula.str = w.wb.xlString(body[2:], int(le16(body, 0)))
			}
		case recHLink:
			if hl := parseHyperLink(body); hl != nil {
				w.HyperLinks = append(w.HyperLinks, hl)
			}
		}
	}
}

func float64frombits(b []byte) float64 {
	return math.Float64frombits(uint64(le32(b, 0)) | uint64(le32(b, 4))<<32)
}

func cellPos(b []byte) Col {
	return Col{RowB: le16(b, 0), FirstColB: le16(b, 2)}
}

func (w *WorkSheet) addMulRK(b []byte) {
	if len(b) < 12 {
		return
	}
	row, first := le16(b, 0), le16(b, 2)
	for i := 0; i < (len(b)-6)/6; i++ {
		col := int(first) + i
		if col > maxCol {
			return
		}
		off := 4 + 6*i
		w.addCell(&RkCol{Col: Col{RowB: row, FirstColB: uint16(col)}, Xfrk: XfRk{Index: le16(b, off), Rk: RK(le32(b, off+2))}})
	}
}

func (w *WorkSheet) addMulBlank(b []byte) {
	if len(b) < 8 {
		return
	}
	row, first := le16(b, 0), le16(b, 2)
	for i := 0; i < (len(b)-6)/2; i++ {
		col := int(first) + i
		if col > maxCol {
			return
		}
		w.addCell(&BlankCol{Col: Col{RowB: row, FirstColB: uint16(col)}, Xf: le16(b, 4+2*i)})
	}
}

type cell interface {
	contentHandler
	Coler
}

func (w *WorkSheet) addCell(c cell) {
	w.wb.cells++
	if c.FirstCol() > maxCol {
		return
	}
	row := w.rows[c.Row()]
	if row == nil {
		row = w.addRow(&rowInfo{Index: c.Row()})
	}
	if row.info.Lcell <= c.LastCol() {
		row.info.Lcell = c.LastCol() + 1
	}
	row.cols[c.FirstCol()] = c
}

func (w *WorkSheet) addRow(info *rowInfo) (row *Row) {
	if info.Index > w.MaxRow {
		w.MaxRow = info.Index
	}
	if row = w.rows[info.Index]; row != nil {
		row.info.Fcell = min(row.info.Fcell, info.Fcell)
		row.info.Lcell = max(row.info.Lcell, info.Lcell)
	} else {
		row = &Row{wb: w.wb, info: info, cols: make(map[uint16]contentHandler)}
		w.rows[info.Index] = row
	}
	return
}

// urlMoniker is the CLSID of a URL moniker, [MS-OSHARED] 2.3.7.
var urlMoniker = [16]byte{0xE0, 0xC9, 0xEA, 0x79, 0xF9, 0xBA, 0xCE, 0x11, 0x8C, 0x82, 0x00, 0xAA, 0x00, 0x4B, 0xA9, 0x0B}

// parseHyperLink reads an HLINK record ([MS-XLS] 2.4.140). Moniker types other
// than URL are not decoded.
func parseHyperLink(b []byte) *HyperLink {
	if len(b) < 32 {
		return nil
	}
	hl := &HyperLink{CellRange: CellRange{le16(b, 0), le16(b, 2), le16(b, 4), le16(b, 6)}}
	flags := le32(b, 28)
	p := 32
	str := func() (string, bool) { // HyperlinkString: u32 count, then UTF-16 with NUL
		if len(b) < p+4 {
			return "", false
		}
		n := int(le32(b, p))
		if n > (len(b)-p-4)/2 {
			return "", false
		}
		u := make([]uint16, n)
		for i := range u {
			u[i] = le16(b, p+4+2*i)
		}
		p += 4 + 2*n
		if n > 0 && u[n-1] == 0 {
			u = u[:n-1]
		}
		return string(utf16.Decode(u)), true
	}
	var ok bool
	if flags&0x10 != 0 {
		if hl.Description, ok = str(); !ok {
			return hl
		}
	}
	if flags&0x80 != 0 {
		if hl.TargetFrame, ok = str(); !ok {
			return hl
		}
	}
	if flags&0x1 != 0 && flags&0x100 == 0 {
		if len(b) < p+20 || [16]byte(b[p:p+16]) != urlMoniker {
			return hl
		}
		n := int(le32(b, p+16))
		p += 20
		if n > len(b)-p {
			return hl
		}
		u := make([]uint16, n/2)
		for i := range u {
			u[i] = le16(b, p+2*i)
		}
		for len(u) > 0 && u[len(u)-1] == 0 {
			u = u[:len(u)-1]
		}
		hl.Url, hl.IsUrl = string(utf16.Decode(u)), true
		p += n
	}
	if flags&0x8 != 0 {
		hl.TextMark, _ = str()
	}
	return hl
}
