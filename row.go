package xls

type rowInfo struct {
	Index    uint16
	Fcell    uint16
	Lcell    uint16
	Height   uint16
	Notused  uint16
	Notused2 uint16
	Flags    uint32
}

// Row the data of one row
type Row struct {
	wb   *WorkBook
	info *rowInfo
	cols map[uint16]contentHandler
}

// Col returns the text of column i, or "" if the cell is missing or i is out of range.
func (r *Row) Col(i int) string {
	if r == nil || i < 0 || i > maxCol {
		return ""
	}
	if ch, ok := r.cols[uint16(i)]; ok {
		if strs := ch.String(r.wb); len(strs) > 0 {
			return strs[0]
		}
	}
	return ""
}

// ColExact is the same as Col; every cell is stored on its own.
func (r *Row) ColExact(i int) string {
	return r.Col(i)
}

// LastCol returns the column index after the last cell of the row, like the
// colMac field of the ROW record.
func (r *Row) LastCol() int {
	if r == nil {
		return 0
	}
	return int(r.info.Lcell)
}

// FirstCol returns the index of the first column of the row.
func (r *Row) FirstCol() int {
	if r == nil {
		return 0
	}
	return int(r.info.Fcell)
}

// values returns all cell texts from column 0.
func (r *Row) values() []string {
	data := make([]string, r.info.Lcell)
	for c := range r.cols {
		if int(c) < len(data) {
			data[c] = r.Col(int(c))
		}
	}
	return data
}
