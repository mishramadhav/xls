# Conformance

Audit of the reader against [MS-CFB] (compound file) and [MS-XLS] (BIFF8).
Status: **compliant** (already right), **fixed** (upstream was wrong; the fix has a test),
**not supported** (deliberate).

## [MS-CFB]

| Area | Status | Notes |
| --- | --- | --- |
| Header signature, byte order, version | fixed | Upstream ignored errors from the header read. Now only version 3 (shift 9) and 4 (shift 12) are accepted; any other combination is an error. |
| Sector size | fixed | Upstream hard-coded 512. Now 512 (v3) or 4096 (v4), sector position `(sid+1) * size`. Test: `TestContainerShapes`. |
| Mini sector size and cutoff | fixed | Mini sector 64 bytes; cutoff must be 4096. Streams under the cutoff use the mini stream. |
| FAT and DIFAT | fixed | Upstream read only the header entries correctly; the DIFAT loop could loop for ever. Now FAT count and DIFAT chain length are checked against the file size. Test: DIFAT case (more than 109 FAT sectors). |
| Chain end, free and special sectors | fixed | Chains end at ENDOFCHAIN; FREESECT, DIFSECT, FATSECT or an index outside the FAT in a chain is an error. |
| Cycle detection | fixed | A chain longer than the FAT is a loop and an error (upstream: endless loop or `log.Fatal`). |
| Mini-FAT | fixed | Upstream read only the first mini-FAT sector, skipped its last entry and ignored the chain. Now the whole chain is followed through the FAT. |
| Directory entries | fixed | Name length must be even and at most 64 bytes, else the entry is ignored. Unallocated entries are skipped, not treated as the end. The root entry must be first. |
| Directory tree | fixed | `Workbook` or `Book` is looked up among the root's direct children through the red-black tree. Names compare without case. |
| Stream size and chain | fixed | A stream whose size exceeds its sector chain is an error. Allocation happens after this check, so it is bounded by the file size. Version 3 uses the low 32 bits of the size only. |
| Truncated last sector | compliant | Data sectors may be short at the end of the file; FAT, DIFAT and directory sectors must be complete. |
| Files over 256 MiB | not supported | The container is read into memory; larger input is an error. |
| Transactions, CLSID, timestamps, red-black colours | not supported | Not needed to read a stream. |

## [MS-XLS] BIFF8

| Area | Status | Notes |
| --- | --- | --- |
| Record header, size up to 8224 | fixed | A larger or truncated record ends the substream with an error (upstream: allocation from unchecked sizes). |
| BOF/EOF nesting, substream type | fixed | The workbook must start with a globals BOF. Globals and sheets end at their own EOF; nested substreams (for example charts) are skipped. Upstream parsed the sheets as part of the globals and stopped a sheet at the first nested EOF. |
| BOUNDSHEET8 | fixed | At most 1024 sheets are kept; later records are ignored (each sheet is parsed again on access). Offset is checked against the stream; the visibility field uses its 2 low bits; the name is a ShortXLUnicodeString. A bad offset sets `WorkSheet.Err()`. |
| CODEPAGE | fixed | Used for BIFF5 byte strings (cp1250 to cp1258; others fall back to cp1252). Upstream always used cp1251. BIFF8 strings are Unicode. |
| BIFF5 (`Book` stream) | compliant | Read for LABEL, BOUNDSHEET, FORMAT, FONT. No SST. Tested with a raw file. |
| SST and CONTINUE | fixed | Character counts continue across records; the fHighByte flag is read again at every split inside character data; rich-text runs and phonetic data are skipped across splits without a flag. A truncated SST, or a truncated CONTINUE after it, is an error. Tests: golden `long_sst` and the raw SST case. |
| EXTSST | compliant | Ignored; it is only an index. |
| XLUnicodeString and ShortXLUnicodeString | fixed | One decoder for both. Short input gives what is available instead of panicking. |
| LABELSST | fixed | An index outside the SST gives `""` (upstream issue #60). |
| LABEL | compliant | |
| NUMBER | fixed | Rendered without exponent; formats come from the cell XF (upstream panicked when the XF or FORMAT was missing). |
| RK | compliant | All four encodings (integer or float, with or without /100) are decoded. Test: `TestConformance`. |
| MULRK, MULBLANK | fixed | Stored per cell. Upstream stored one entry and scanned the row; a short span panicked. Spans stop at column 255. |
| BLANK | compliant | |
| BOOLERR | fixed | Was ignored. Now TRUE, FALSE or the error name (`#N/A`, `#DIV/0!`, ...). |
| FORMULA and STRING | fixed | Upstream never stored formula cells and returned the text `FormulaCol`. Now the cached number, boolean, error or string is returned. The STRING record may follow ARRAY or SHRFMLA records. The formula itself is not evaluated. |
| ROW and DIMENSIONS | fixed | Both are optional. `Row.LastCol` is the column after the last cell for all rows, also when the ROW record follows its cells (upstream mixed this and the last column). DIMENSIONS is not used. |
| Column and row limits | fixed | Cells with a column above 255 are ignored. Row access with an index outside 0..65535 returns nil (upstream wrapped the index). |
| DATEMODE (1904) | fixed | Honoured. Upstream's date helper was wrong for serials below 61 in the 1900 system; serial 60 (the non-existent 1900-02-29) maps to 1900-03-01. |
| XF and FORMAT | fixed | Only the number format index is used. Built-in date formats 14-22, 27-36, 45-47 and 50-58 give RFC 3339 text. A custom format is a date format if it has a d, m, y, h or s code outside quotes and brackets (upstream used substring guesses). It is rendered with a Go layout built from yyyy, yy, mmmm, mmm, mm, m, dddd, ddd, dd, d, hh, h, ss, s and AM/PM (m is a minute next to h or s); a code with other text or a fraction gives `2006-01-02` or `2006-01-02 15:04:05`. Other formats give the plain number; the format is not applied (no `0.00`, `%` or currency text). Serials outside 0..2958465 stay plain numbers. |
| HLINK | fixed | Flags decoded correctly, counts bounded by the record. Only URL monikers are decoded. Hyperlinks no longer replace cell text. |
| Charts, drawings, comments, merged cells, styles, protection, RSTRING | not supported | Not needed to read cell values. |
| Encrypted workbooks (FilePass) | not supported | `Open` returns an error. |
