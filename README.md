# xls

Pure Go reader for legacy `.xls` files (BIFF8 in a compound file).

This is a hardened fork of [extrame/xls](https://github.com/extrame/xls), which was
based on libxls. Thanks to the original authors and contributors.
Module path: `github.com/mishramadhav/xls`.

## Usage

* `Open(file, charset)` opens a file. It reads the file fully and closes it.
* `OpenWithCloser(file, charset)` does the same and also returns the file as an `io.Closer`.
* `OpenReader(reader, charset)` opens from an `io.ReadSeeker`. The `charset` argument is unused.
* `wb.GetSheet(i)` returns a sheet, `sheet.Row(i)` a row (or nil), `row.Col(i)` the cell text.
  `sheet.Err()` reports a damaged sheet. Cells read before the damage stay available.

```go
wb, err := xls.Open("report.xls", "utf-8")
if err != nil { /* not a valid xls */ }
sheet := wb.GetSheet(0)
for r := 0; r <= int(sheet.MaxRow); r++ {
	row := sheet.Row(r) // nil for a row with no cells
	fmt.Println(row.Col(0), row.Col(1)) // nil-safe
}
```

## What changed in this fork

* Go module, no `tealeg/xlsx` dependency (the xls/xlsx compare helper is gone).
* The `ole2` dependency is now `internal/ole2` (Apache-2.0, see its `LICENSE` and `NOTICE`),
  rewritten to follow [MS-CFB] and to return errors instead of panicking or calling `log.Fatal`.
* Every offset, length and index read from a file is checked. Open never panics:
  residual panics are returned as errors. `GetSheet` does the same and sets `Sheet.Err()`.
* Shared string table: correct CONTINUE handling (flag byte at each split, rich text and
  phonetic data). An out-of-range LABELSST index gives `""` (upstream issue #60).
* Formula cells return their cached value, BOOLERR cells return TRUE/FALSE/error text,
  MULRK and MULBLANK cells are stored per cell.
* Numbers never use an exponent. Dates use the XF number format and the DATEMODE (1900/1904) flag.
* Tests: golden files made with xlwt and read back with xlrd, raw BIFF/CFB cases, and a fuzz target.
* Removed: `WorkBook.Parse`, `MulrkCol`, `MulBlankCol`, `FormulaStringCol`, `HyperLink.String`.
  Hyperlinks are listed in `WorkSheet.HyperLinks` and no longer replace cell text.

See [SPEC.md](SPEC.md) for the conformance table.

## Tests

```
go test -race ./...
go test -run '^$' -fuzz FuzzOpen -fuzztime 30s .
uv run testdata/golden/gen.py   # regenerate the golden files (xlwt, xlrd)
```
