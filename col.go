package xls

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// contentHandler is the content of one cell.
type contentHandler interface {
	String(*WorkBook) []string
	FirstCol() uint16
	LastCol() uint16
}

type Col struct {
	RowB      uint16
	FirstColB uint16
}

type Coler interface {
	Row() uint16
}

func (c *Col) Row() uint16 {
	return c.RowB
}

func (c *Col) FirstCol() uint16 {
	return c.FirstColB
}

func (c *Col) LastCol() uint16 {
	return c.FirstColB
}

func (c *Col) String(wb *WorkBook) []string {
	return []string{"default"}
}

// formatNo returns the number format index of XF record xf.
func (wb *WorkBook) formatNo(xf uint16) (uint16, bool) {
	if wb == nil || int(xf) >= len(wb.Xfs) {
		return 0, false
	}
	return wb.Xfs[xf].formatNo(), true
}

// isDateFormat reports whether a custom format string shows dates or times:
// after removing quoted text, [..] sections and escaped characters it holds a
// d, m, y, h or s code. "General" is not a date.
func isDateFormat(f string) bool {
	var sb strings.Builder
	quoted, bracket, esc := false, false, false
	for _, r := range f {
		switch {
		case esc:
			esc = false
		case r == '\\' || r == '_' || r == '*':
			esc = true
		case r == '"':
			quoted = !quoted
		case quoted:
		case r == '[':
			bracket = true
		case r == ']':
			bracket = false
		case !bracket:
			sb.WriteRune(r)
		}
	}
	return strings.ContainsAny(strings.ToLower(sb.String()), "dmyhs") && !strings.EqualFold(f, "general")
}

func isBuiltinDate(n uint16) bool {
	return 14 <= n && n <= 22 || 27 <= n && n <= 36 || 45 <= n && n <= 47 || 50 <= n && n <= 58
}

// number renders a numeric cell: a date or time text when its XF has a date
// format, else the plain decimal number without exponent.
func (wb *WorkBook) number(xf uint16, f float64, plain string) string {
	n, ok := wb.formatNo(xf)
	if !ok || f < 0 || f >= 2958466 { // Excel dates end at 9999-12-31
		return plain
	}
	switch {
	case isBuiltinDate(n):
		return timeFromExcelTime(f, wb.dateMode == 1).Format(time.RFC3339)
	case n >= 164:
		if fm := wb.Formats[n]; fm != nil && isDateFormat(fm.str) {
			return formatDate(timeFromExcelTime(f, wb.dateMode == 1), fm.str)
		}
	}
	return plain
}

func plainFloat(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

type XfRk struct {
	Index uint16
	Rk    RK
}

func (xf *XfRk) String(wb *WorkBook) string {
	i, f, isFloat := xf.Rk.number()
	if !isFloat {
		f = float64(i)
	}
	return wb.number(xf.Index, f, xf.Rk.String())
}

// RK is the 30-bit number of an RK record, [MS-XLS] 2.5.217.
type RK uint32

func (rk RK) number() (intNum int64, floatNum float64, isFloat bool) {
	div100 := rk&1 != 0
	if rk&2 == 0 { // upper 30 bits of an IEEE double
		floatNum = math.Float64frombits(uint64(rk&^3) << 32)
		if div100 {
			floatNum /= 100
		}
		return 0, floatNum, true
	}
	val := int32(rk) >> 2
	if div100 {
		return 0, float64(val) / 100, true
	}
	return int64(val), 0, false
}

func (rk RK) String() string {
	i, f, isFloat := rk.number()
	if isFloat {
		return plainFloat(f)
	}
	return strconv.FormatInt(i, 10)
}

var ErrIsInt = fmt.Errorf("is int")

func (rk RK) Float() (float64, error) {
	_, f, isFloat := rk.number()
	if !isFloat {
		return 0, ErrIsInt
	}
	return f, nil
}

type NumberCol struct {
	Col
	Index uint16
	Float float64
}

func (c *NumberCol) String(wb *WorkBook) []string {
	return []string{wb.number(c.Index, c.Float, plainFloat(c.Float))}
}

// FormulaCol is a FORMULA record; String returns its cached result.
type FormulaCol struct {
	Header struct {
		Col
		IndexXf uint16
		Result  [8]byte
		Flags   uint16
		_       uint32
	}
	Bts []byte
	str string
}

var errorNames = map[byte]string{
	0x00: "#NULL!", 0x07: "#DIV/0!", 0x0F: "#VALUE!", 0x17: "#REF!",
	0x1D: "#NAME?", 0x24: "#NUM!", 0x2A: "#N/A",
}

func boolErr(v byte, isErr bool) string {
	switch {
	case isErr:
		return errorNames[v]
	case v != 0:
		return "TRUE"
	}
	return "FALSE"
}

func (c *FormulaCol) String(wb *WorkBook) []string {
	r := c.Header.Result
	if r[6] == 0xFF && r[7] == 0xFF { // not a number: type in the first byte
		switch r[0] {
		case 0:
			return []string{c.str}
		case 1:
			return []string{boolErr(r[2], false)}
		case 2:
			return []string{boolErr(r[2], true)}
		}
		return []string{""}
	}
	f := float64frombits(r[:])
	return []string{wb.number(c.Header.IndexXf, f, plainFloat(f))}
}

type RkCol struct {
	Col
	Xfrk XfRk
}

func (c *RkCol) String(wb *WorkBook) []string {
	return []string{c.Xfrk.String(wb)}
}

type LabelsstCol struct {
	Col
	Xf  uint16
	Sst uint32
}

// String returns "" for an index outside the shared string table.
func (c *LabelsstCol) String(wb *WorkBook) []string {
	if uint64(c.Sst) < uint64(len(wb.sst)) {
		return []string{wb.sst[c.Sst]}
	}
	return []string{""}
}

type labelCol struct {
	BlankCol
	Str string
}

func (c *labelCol) String(wb *WorkBook) []string {
	return []string{c.Str}
}

type BlankCol struct {
	Col
	Xf uint16
}

func (c *BlankCol) String(wb *WorkBook) []string {
	return []string{""}
}

// BoolErrCol is a BOOLERR record: TRUE, FALSE or an error name such as #N/A.
type BoolErrCol struct {
	Col
	Xf    uint16
	Val   byte
	IsErr bool
}

func (c *BoolErrCol) String(wb *WorkBook) []string {
	return []string{boolErr(c.Val, c.IsErr)}
}

func (c *FormulaCol) Row() uint16      { return c.Header.RowB }
func (c *FormulaCol) FirstCol() uint16 { return c.Header.FirstColB }
func (c *FormulaCol) LastCol() uint16  { return c.Header.FirstColB }
