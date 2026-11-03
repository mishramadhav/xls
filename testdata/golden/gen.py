# /// script
# requires-python = ">=3.9"
# dependencies = ["xlwt==1.3.0", "xlrd==2.0.2"]
# ///
"""Writes the synthetic .xls fixtures (xlwt, BIFF8) and the expected cell text
(read back with xlrd) next to this script.

    uv run testdata/golden/gen.py
"""
import json
import random
from decimal import Decimal
from pathlib import Path

import xlrd
import xlwt

HERE = Path(__file__).parent


def num_text(v):
    s = format(Decimal(repr(float(v))), "f")
    return s.rstrip("0").rstrip(".") if "." in s else s


def cell_text(book, cell):
    t = cell.ctype
    if t in (xlrd.XL_CELL_EMPTY, xlrd.XL_CELL_BLANK):
        return ""
    if t == xlrd.XL_CELL_NUMBER:
        return num_text(cell.value)
    if t == xlrd.XL_CELL_BOOLEAN:
        return "TRUE" if cell.value else "FALSE"
    if t == xlrd.XL_CELL_ERROR:
        return xlrd.error_text_from_code[cell.value]
    return cell.value


def dump(path):
    book = xlrd.open_workbook(str(path))
    sheets = []
    for sh in book.sheets():
        rows = [[cell_text(book, sh.cell(r, c)) for c in range(sh.ncols)] for r in range(sh.nrows)]
        sheets.append({"name": sh.name, "rows": rows})
    path.with_suffix(".json").write_text(json.dumps({"sheets": sheets}, ensure_ascii=False, separators=(",", ":")))


def blank_rows():
    wb = xlwt.Workbook()
    ws = wb.add_sheet("Blank")
    for c, h in enumerate(["id", "name", "amount"]):
        ws.write(5, c, h)  # rows 0-4 have no cells at all
    for i in range(8, 20):  # rows 6-7 have no cells either
        ws.write(i, 0, i)
        ws.write(i, 1, "row %d" % i)
        ws.write(i, 2, i * 1.5)
    wb.save(str(HERE / "blank_rows.xls"))


def long_sst():
    rng = random.Random(7)
    alpha = "abcdefghijklmnopqrstuvwxyz0123456789 -_"
    wide = "éüñßøåÅЖжЩ日本語テキスト한국어😀𝄞"
    wb = xlwt.Workbook()
    ws = wb.add_sheet("Strings")
    for i in range(6000):
        n = rng.randint(1, 60)
        pool = alpha + (wide if i % 3 == 0 else "")
        s = "%05d-" % i + "".join(rng.choice(pool) for _ in range(n))
        ws.write(i, 0, s)
        ws.write(i, 1, i)
    for k in range(3):  # strings far longer than one record
        ws.write(6000 + k, 0, ("long%d-" % k) + "é日" * 9000 if k == 1 else ("long%d-" % k) + "x" * 20000)
    wb.save(str(HERE / "long_sst.xls"))


def numbers():
    wb = xlwt.Workbook()
    ws = wb.add_sheet("Numbers")
    vals = [0, 1, -1, 42, 536870911, 536870912, -536870912, 7016631401, 1216000, 0.1, 1.25, -1.25,
            123.45, 0.01, 99.99, 1e15, 1234567890123.5, 0.000123, 123456789012345678, 3.141592653589793,
            -0.5, 100, 1e-7]
    for i, v in enumerate(vals):
        ws.write(i, 0, v)
        ws.write(i, 1, str(v))
    r = len(vals)
    ws.write(r, 0, True)
    ws.write(r, 1, False)
    ws.row(r + 1).set_cell_error(0, 0x2A)  # #N/A
    ws.row(r + 1).set_cell_error(1, 0x07)  # #DIV/0!
    ws.write(r + 2, 0, xlwt.Formula("1+2"))
    ws.write(r + 3, 0, "")  # empty string -> BLANK
    ws.write(r + 3, 2, "after blank")
    wb.save(str(HERE / "numbers.xls"))


def texts():
    wb = xlwt.Workbook()
    ws = wb.add_sheet("Report")
    ws2 = wb.add_sheet("Zweite Тест")
    rows = [
        ["'2026-10-16", "16-10-2026", "2026-10-16 09:30:00", "'007016631401", "xid"],
        ["'2026-10-17", "17/10/2026", "Oct 17, 2026", "'0000123", "9m4e2mr0ui3e8a215n4g"],
        ["tab\tand\nnewline", "quote\"s", "ünïcode", "日本語", "😀"],
    ]
    for r, row in enumerate(rows):
        for c, v in enumerate(row):
            ws.write(r, c, v)
    ws2.write(0, 0, "second")
    ws2.write(2, 3, 7016631401)
    wb.save(str(HERE / "texts.xls"))


if __name__ == "__main__":
    for f in (blank_rows, long_sst, numbers, texts):
        f()
    for p in sorted(HERE.glob("*.xls")):
        dump(p)
