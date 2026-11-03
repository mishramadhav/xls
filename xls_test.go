package xls

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type goldenSheet struct {
	Name string     `json:"name"`
	Rows [][]string `json:"rows"`
}

// TestGolden compares every cell with the text xlrd read from the same file
// (see testdata/golden/gen.py).
func TestGolden(t *testing.T) {
	files, _ := filepath.Glob("testdata/golden/*.xls")
	if len(files) == 0 {
		t.Fatal("no golden fixtures")
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			raw, err := os.ReadFile(f[:len(f)-4] + ".json")
			if err != nil {
				t.Fatal(err)
			}
			var want struct{ Sheets []goldenSheet }
			if err := json.Unmarshal(raw, &want); err != nil {
				t.Fatal(err)
			}
			wb, err := Open(f, "utf-8")
			if err != nil {
				t.Fatal(err)
			}
			if wb.NumSheets() != len(want.Sheets) {
				t.Fatalf("sheets = %d, want %d", wb.NumSheets(), len(want.Sheets))
			}
			for i, ws := range want.Sheets {
				sh := wb.GetSheet(i)
				if sh.Err() != nil {
					t.Fatal(sh.Err())
				}
				if sh.Name != ws.Name {
					t.Errorf("sheet %d name = %q, want %q", i, sh.Name, ws.Name)
				}
				compareSheet(t, sh, ws.Rows)
			}
		})
	}
}

// compareSheet checks every expected cell and that no cell exists outside the expected grid.
func compareSheet(t *testing.T, sh *WorkSheet, want [][]string) {
	t.Helper()
	bad := 0
	for r, row := range want {
		for c, w := range row {
			if got := sh.Row(r).Col(c); got != w {
				if bad++; bad <= 10 {
					t.Errorf("%s r%d c%d = %q, want %q", sh.Name, r, c, got, w)
				}
			}
		}
		if got := sh.Row(r).LastCol(); got > len(row) {
			t.Errorf("%s r%d LastCol = %d, want <= %d", sh.Name, r, got, len(row))
		}
	}
	if len(want) > 0 && int(sh.MaxRow) >= len(want) {
		t.Errorf("%s MaxRow = %d, want < %d", sh.Name, sh.MaxRow, len(want))
	}
}

// TestOpenTestdata opens the older sample files: they must parse without error.
func TestOpenTestdata(t *testing.T) {
	files, _ := filepath.Glob("testdata/*.xls")
	files = append(files, "Table.xls", "BigTable.xls")
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			wb, err := Open(f, "utf-8")
			if err != nil {
				t.Fatal(err)
			}
			for i := 0; i < wb.NumSheets(); i++ {
				if err := wb.GetSheet(i).Err(); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
