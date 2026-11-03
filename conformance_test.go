package xls

import (
	"bytes"
	"encoding/binary"
	"math"
	"runtime"
	"strconv"
	"testing"
	"time"
)

func rkInt(v int32, div100 bool) uint32 {
	rk := uint32(v<<2) | 2
	if div100 {
		rk |= 1
	}
	return rk
}

func rkFloat(f float64, div100 bool) uint32 {
	rk := uint32(math.Float64bits(f) >> 32)
	if div100 {
		rk |= 1
	}
	return rk
}

func openRaw(t *testing.T, stream []byte, ss int) *WorkBook {
	t.Helper()
	wb, err := OpenReader(bytes.NewReader(cfbFile(stream, ss)), "")
	if err != nil {
		t.Fatal(err)
	}
	return wb
}

func result(typ, val byte) (r [8]byte) {
	r[0], r[2], r[6], r[7] = typ, val, 0xFF, 0xFF
	return
}

func sstWithSegments(unique uint32, segs ...[]byte) []byte {
	out := rec(0xFC, u32s(unique, unique), segs[0])
	for _, s := range segs[1:] {
		out = cat(out, rec(0x3C, s))
	}
	return out
}

// TestConformance covers [MS-XLS] record types xlwt does not write, each
// through a raw workbook stream.
func TestConformance(t *testing.T) {
	sstCases := sstWithSegments(7,
		cat(u16s(6), []byte{0}, []byte("abc")),
		cat([]byte{1}, utf16le("日é!"), u16s(5), []byte{1}, utf16le("日本")),
		cat([]byte{0}, []byte("xyz"), u16s(2), []byte{0}, []byte("ok"), u16s(2), []byte{8}, u16s(2), []byte("hi"), []byte{1, 2, 3}),
		cat([]byte{4, 5, 6, 7, 8}, u16s(1), []byte{4}, u32s(10), []byte("Z"), []byte{1, 2, 3, 4}),
		cat([]byte{5, 6, 7, 8, 9, 10}, u16s(3), []byte{0}),
		cat([]byte{0}, []byte("xyz"), u16s(4), []byte{0}, []byte("last")),
	)
	row := make([]string, 256)
	row[254], row[255] = "1", "2"

	cases := []struct {
		name    string
		globals [][]byte
		sheet   [][]byte
		want    [][]string
	}{
		{
			name: "RK four encodings, MULRK, MULBLANK",
			sheet: [][]byte{
				rkRec(0, 0, 0, rkInt(5, false)), rkRec(0, 1, 0, rkInt(12345, true)),
				rkRec(0, 2, 0, rkFloat(1.5, false)), rkRec(0, 3, 0, rkFloat(150, true)),
				rkRec(0, 4, 0, rkInt(-7, false)), rkRec(0, 5, 0, rkInt(-250, true)),
				mulRKRec(1, 2, rkInt(1, false), rkInt(250, true), rkFloat(0.5, false)),
				mulBlankRec(2, 1, 3), labelRec(2, 4, "end"),
			},
			want: [][]string{
				{"5", "123.45", "1.5", "1.5", "-7", "-2.5"},
				{"", "", "1", "2.5", "0.5"},
				{"", "", "", "", "end"},
			},
		},
		{
			name: "BOOLERR and FORMULA cached values",
			sheet: [][]byte{
				boolErrRec(0, 0, 1, 0), boolErrRec(0, 1, 0, 0), boolErrRec(0, 2, 0x2A, 1), boolErrRec(0, 3, 0x07, 1),
				formulaRec(1, 0, 0, [8]byte(f64(2.5))),
				formulaRec(1, 1, 0, result(0, 0)), rec(0x221, make([]byte, 12)), stringRec("hello"),
				formulaRec(1, 2, 0, result(1, 1)), formulaRec(1, 3, 0, result(2, 0x0F)), formulaRec(1, 4, 0, result(3, 0)),
			},
			want: [][]string{{"TRUE", "FALSE", "#N/A", "#DIV/0!"}, {"2.5", "hello", "TRUE", "#VALUE!", ""}},
		},
		{
			name:  "LABEL record and LABELSST outside the table",
			sheet: [][]byte{labelRec(0, 0, "plain"), sstIdxRec(0, 1, 0), sstIdxRec(0, 2, 0xFFFFFFFF)},
			want:  [][]string{{"plain", "", ""}},
		},
		{
			name:    "SST CONTINUE: fHighByte switches, rich text, phonetic data, split at header",
			globals: [][]byte{sstCases},
			sheet: [][]byte{
				sstIdxRec(0, 0, 0), sstIdxRec(0, 1, 1), sstIdxRec(0, 2, 2), sstIdxRec(0, 3, 3),
				sstIdxRec(0, 4, 4), sstIdxRec(0, 5, 5), sstIdxRec(0, 6, 6),
			},
			want: [][]string{{"abc日é!", "日本xyz", "ok", "hi", "Z", "xyz", "last"}},
		},
		{
			name: "date formats, 1900 system",
			globals: [][]byte{formatRec(164, "yyyy-mm-dd"), formatRec(165, `0.0 "days"`), formatRec(166, "#,##0.00"),
				xfRec(0), xfRec(14), xfRec(164), xfRec(165), xfRec(166), xfRec(2)},
			sheet: [][]byte{
				numberRec(0, 0, 0, 44197), numberRec(0, 1, 1, 44197), numberRec(0, 2, 2, 44197),
				numberRec(0, 3, 3, 44197), numberRec(0, 4, 4, 44197), numberRec(0, 5, 5, 44197.5),
				rkRec(1, 0, 1, rkInt(44197, false)), numberRec(1, 1, 1, -5), numberRec(1, 2, 99, 44197),
				numberRec(2, 0, 1, 1), numberRec(2, 1, 1, 59), numberRec(2, 2, 1, 61), numberRec(2, 3, 1, 36526.75),
			},
			want: [][]string{
				{"44197", "2021-01-01T00:00:00Z", "2021-01-01", "44197", "44197", "44197.5"},
				{"2021-01-01T00:00:00Z", "-5", "44197"},
				{"1900-01-01T00:00:00Z", "1900-02-28T00:00:00Z", "1900-03-01T00:00:00Z", "2000-01-01T18:00:00Z"},
			},
		},
		{
			name:    "date formats, 1904 system",
			globals: [][]byte{rec(0x22, u16s(1)), xfRec(0), xfRec(14)},
			sheet:   [][]byte{numberRec(0, 0, 1, 44197)},
			want:    [][]string{{"2025-01-02T00:00:00Z"}},
		},
		{
			name: "cells and spans outside the 256 columns are dropped",
			sheet: [][]byte{
				numberRec(0, 0, 0, 1), numberRec(0, 256, 0, 2), numberRec(0, 65535, 0, 3),
				mulRKRec(1, 254, rkInt(1, false), rkInt(2, false), rkInt(3, false)),
			},
			want: [][]string{{"1"}, row},
		},
	}
	for _, tc := range cases {
		for _, ss := range []int{512, 4096} {
			t.Run(tc.name+"/"+strconv.Itoa(ss), func(t *testing.T) {
				wb := openRaw(t, xlsFile(tc.globals, tc.sheet...), ss)
				sh := wb.GetSheet(0)
				if sh.Err() != nil {
					t.Fatal(sh.Err())
				}
				for r, row := range tc.want {
					for c, w := range row {
						if got := sh.Row(r).Col(c); got != w {
							t.Errorf("r%d c%d = %q, want %q", r, c, got, w)
						}
					}
				}
			})
		}
	}
}

// TestContainerShapes runs one workbook through every container layout:
// mini stream, regular stream, version 4 sectors and a DIFAT chain.
func TestContainerShapes(t *testing.T) {
	small := xlsFile(nil, labelRec(0, 0, "ok"))
	pad := func(n int) []byte { return cat(small, make([]byte, n)) }
	cases := []struct {
		name   string
		stream []byte
		ss     int
	}{
		{"mini stream v3", small, 512},
		{"mini stream v4", small, 4096},
		{"regular stream v3", pad(6000), 512},
		{"regular stream v4", pad(20000), 4096},
		{"DIFAT chain v3", pad(8 << 20), 512},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wb := openRaw(t, tc.stream, tc.ss)
			if got := wb.GetSheet(0).Row(0).Col(0); got != "ok" {
				t.Fatalf("cell = %q", got)
			}
		})
	}
}

// TestMalformed feeds damaged files to the public API: it must return errors
// or partial data, never panic.
func TestMalformed(t *testing.T) {
	good := xlsFile(nil, labelRec(0, 0, "ok"))
	file := func() []byte { return cfbFile(cat(good, make([]byte, 6000)), 512) }
	mut := func(f func(b []byte)) []byte { b := file(); f(b); return b }
	cases := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{"empty", nil, true},
		{"short header", file()[:100], true},
		{"bad signature", mut(func(b []byte) { b[0] = 0 }), true},
		{"bad sector shift", mut(func(b []byte) { b[30] = 10 }), true},
		{"bad mini cutoff", mut(func(b []byte) { b[56] = 1 }), true},
		{"FAT count beyond file", mut(func(b []byte) { copy(b[44:], u32s(0xFFFFFF)) }), true},
		{"directory start out of range", mut(func(b []byte) { copy(b[48:], u32s(0x7FFFFFF0)) }), true},
		{"directory chain loops", mut(func(b []byte) { copy(b[48:], u32s(0)) }), true},
		{"truncated file", file()[:2000], true},
		{"workbook stream missing", mut(func(b []byte) { copy(b[bytes.Index(b, utf16le("Workbook")):], utf16le("Nope")) }), true},
		{"not a BIFF stream", cfbFile(make([]byte, 100), 512), true},
		{"record larger than 8224", cfbFile(xlsFile([][]byte{u16s(0x99, 9000), make([]byte, 9000)}), 512), true},
		{"encrypted workbook", cfbFile(xlsFile([][]byte{rec(0x2F, make([]byte, 6))}), 512), true},
		{"truncated CONTINUE after SST", cfbFile(xlsFile([][]byte{rec(0xFC, u32s(1, 1), []byte{1, 0, 0}), u16s(0x3C, 5000)}), 512), true},
		{"SST count beyond data", cfbFile(xlsFile([][]byte{rec(0xFC, u32s(1, 0xFFFFFFF0), make([]byte, 10))}), 512), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wb, err := OpenReader(bytes.NewReader(tc.data), "")
			if tc.wantErr != (err != nil) {
				t.Fatalf("err = %v, want error: %v", err, tc.wantErr)
			}
			if wb != nil {
				touchAll(wb)
			}
		})
	}

	// ss 4096: one FAT sector holds 1024 entries, so a 1024-sector chain is
	// declared by a ~100 KB file. FAT sid is at header offset 76.
	big := func() []byte { return cfbFile(cat(good, make([]byte, 100000)), 4096) }
	fatEntry := func(b []byte, i int) []byte {
		return b[(int(binary.LittleEndian.Uint32(b[76:]))+1)*4096+4*i:]
	}
	t.Run("FAT chain past the last sector", func(t *testing.T) {
		b := big()
		copy(fatEntry(b, 0), u32s(500))
		copy(fatEntry(b, 500), u32s(eoc))
		if _, err := OpenReader(bytes.NewReader(b), ""); err == nil {
			t.Error("want an error")
		}
	})
	t.Run("huge stream size on a long FAT chain", func(t *testing.T) {
		b := big()
		for i := 0; i < 1023; i++ {
			copy(fatEntry(b, i), u32s(uint32(i+1)))
		}
		copy(fatEntry(b, 1023), u32s(eoc))
		dir := (int(binary.LittleEndian.Uint32(b[48:])) + 1) * 4096
		binary.LittleEndian.PutUint64(b[dir+128+120:], 1024*4096)
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		_, err := OpenReader(bytes.NewReader(b), "")
		runtime.ReadMemStats(&after)
		if err == nil {
			t.Error("want an error")
		}
		if d := after.TotalAlloc - before.TotalAlloc; d > 8*uint64(len(b)) {
			t.Errorf("allocated %d bytes for a %d byte file", d, len(b))
		}
	})

	t.Run("ROW after its cells keeps the cells", func(t *testing.T) {
		wb := openRaw(t, xlsFile(nil, labelRec(0, 0, "a"), labelRec(0, 3, "d"), rec(0x208, u16s(0, 0, 1, 0, 0, 0), u32s(0))), 512)
		if got := wb.ReadAllCells(10); len(got) != 1 || len(got[0]) != 4 || got[0][3] != "d" {
			t.Errorf("ReadAllCells = %q, want a row with column 3", got)
		}
	})

	t.Run("sheet count is capped", func(t *testing.T) {
		var bs []byte
		for range 3000 {
			bs = cat(bs, rec(0x85, u32s(uint32(1<<20+len(bs))), []byte{0, 0, 1, 0}, []byte("S")))
		}
		wb, err := OpenReader(bytes.NewReader(cfbFile(cat(bofRec(0x600, 5), bs, rec(0x0A)), 512)), "")
		if err != nil {
			t.Fatal(err)
		}
		if n := wb.NumSheets(); n != maxSheets {
			t.Errorf("NumSheets = %d, want %d", n, maxSheets)
		}
	})

	t.Run("repeated or in-globals sheet offsets are ignored", func(t *testing.T) {
		head := bofRec(0x600, 5)
		bs := func(pos uint32) []byte { return rec(0x85, u32s(pos), []byte{0, 0, 1, 0}, []byte("S")) }
		// 3 records at offset 1<<20, 1 at offset 0 (inside the globals), 1 at the globals' own BOF.
		stream := cat(head, bs(1<<20), bs(1<<20), bs(1<<20), bs(0), bs(uint32(len(head))), rec(0x0A))
		wb, err := OpenReader(bytes.NewReader(cfbFile(stream, 512)), "")
		if err != nil {
			t.Fatal(err)
		}
		if n := wb.NumSheets(); n != 1 {
			t.Errorf("NumSheets = %d, want 1", n)
		}
	})

	t.Run("cell total is capped", func(t *testing.T) {
		blanks := mulBlankRec(0, 0, 256)
		recs := make([][]byte, maxCells/256+2)
		for i := range recs {
			recs[i] = blanks
		}
		wb := openRaw(t, xlsFile(nil, recs...), 512)
		if wb.GetSheet(0).Err() == nil {
			t.Error("want a sheet error after the cell limit")
		}
	})

	t.Run("sheet offset beyond stream", func(t *testing.T) {
		wb := openRaw(t, xlsFile(nil, labelRec(0, 0, "x"))[:45], 512)
		if wb.GetSheet(0).Err() == nil {
			t.Error("want a sheet error")
		}
	})

	t.Run("sheet problems", func(t *testing.T) {
		huge := rec(0x1B8, u16s(0, 65535, 0, 255), make([]byte, 16), u32s(2, 0x10|1|8), u32s(0x7FFFFFFF))
		wb := openRaw(t, xlsFile(nil, huge, mulRKRec(0, 0, 1), rec(0xBD, []byte{1}), rec(0x208, []byte{1}), labelRec(0, 0, "x")), 512)
		touchAll(wb)
		if wb.GetSheet(-1) != nil || wb.GetSheet(9) != nil {
			t.Error("GetSheet out of range must be nil")
		}
		if wb.GetSheet(0).Row(-1) != nil || wb.GetSheet(0).Row(65536) != nil || wb.GetSheet(0).Row(0).Col(-1) != "" {
			t.Error("out-of-range indexes must give nil row or empty text")
		}
		if got := wb.GetSheet(0).Row(0).Col(0); got != "x" {
			t.Errorf("cell = %q", got)
		}
		var nilRow *Row
		if nilRow.Col(0) != "" || nilRow.LastCol() != 0 {
			t.Error("nil row must be safe")
		}
	})
}

// touchAll reads everything the public API exposes.
func touchAll(wb *WorkBook) {
	wb.ReadAllCells(1 << 20)
	for i := 0; i < wb.NumSheets(); i++ {
		s := wb.GetSheet(i)
		for r := 0; r <= int(s.MaxRow); r++ {
			row := s.Row(r)
			for c := 0; c <= row.LastCol()+1; c++ {
				row.Col(c)
			}
		}
	}
}

func TestBIFF5(t *testing.T) {
	bs := func(pos uint32) []byte { return rec(0x85, u32s(pos), []byte{0, 0, 3}, []byte("Old")) }
	head := cat(bofRec(0x500, 5), rec(0x42, u16s(1251)))
	end := rec(0x0A)
	pos := uint32(len(head) + len(bs(0)) + len(end))
	label := rec(0x204, u16s(0, 0, 0, 6), []byte{0xCF, 0xF0, 0xE8, 0xE2, 0xE5, 0xF2})
	stream := cat(head, bs(pos), end, bofRec(0x500, 0x10), label, rec(0x0A))
	wb, err := OpenReader(bytes.NewReader(cfbFileNamed(stream, "Book")), "")
	if err != nil {
		t.Fatal(err)
	}
	if got := wb.GetSheet(0).Row(0).Col(0); got != "Привет" || wb.GetSheet(0).Name != "Old" {
		t.Errorf("got %q in sheet %q", got, wb.GetSheet(0).Name)
	}
}

func cfbFileNamed(stream []byte, name string) []byte {
	b := cfbFile(stream, 512)
	// directory is the sector after the one mini-stream and mini-FAT sector
	i := bytes.Index(b, utf16le("Workbook"))
	copy(b[i:], utf16le(name))
	copy(b[i+2*len(name):], make([]byte, 2*(8-len(name))+2))
	b[i+64] = byte(len(name)*2 + 2)
	return b
}

func TestGoLayout(t *testing.T) {
	cases := []struct{ code, want string }{
		{"yyyy-mm-dd", "2006-01-02"},
		{"dd/mm/yy", "02/01/06"},
		{"d-mmm-yy", "2-Jan-06"},
		{"mmmm d, yyyy", "January 2, 2006"},
		{"dddd, mmm d", "Monday, Jan 2"},
		{"hh:mm:ss", "15:04:05"},
		{"h:mm AM/PM", "3:04 PM"},
		{"hh:mm:ss am/pm", "03:04:05 PM"},
		{"yyyy-mm-dd hh:mm:ss", "2006-01-02 15:04:05"},
		{"mm:ss", "04:05"},
		{"m/d/yyyy h:mm", "1/2/2006 15:04"},
		{"[$-409]yyyy-mm-dd", "2006-01-02"},
		{`yyyy"T"`, ""},
		{"yyyy-mm-dd;@", ""},
		{"hh:mm:ss.00", ""},
	}
	for _, tc := range cases {
		t.Run(tc.code, func(t *testing.T) {
			if got := goLayout(tc.code); got != tc.want {
				t.Errorf("goLayout(%q) = %q, want %q", tc.code, got, tc.want)
			}
		})
	}
}

func TestFormatDate(t *testing.T) {
	ts := time.Date(2021, 1, 2, 15, 4, 5, 0, time.UTC)
	cases := []struct {
		name string
		t    time.Time
		code string
		want string
	}{
		{"mapped", ts, "dd/mm/yyyy", "02/01/2021"},
		{"unmapped with time", ts, `yyyy"x"`, "2021-01-02 15:04:05"},
		{"unmapped date only", ts.Truncate(24 * time.Hour), `yyyy"x"`, "2021-01-02"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatDate(tc.t, tc.code); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
