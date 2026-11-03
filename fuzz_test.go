package xls

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime/pprof"
	"testing"
	"time"
)

// FuzzOpen runs the reader on arbitrary bytes. It calls the unrecovered
// internals so that a residual panic is a failure here, not a returned error.
func FuzzOpen(f *testing.F) {
	for _, pat := range []string{"testdata/golden/*.xls", "testdata/*.xls", "*.xls"} {
		files, _ := filepath.Glob(pat)
		for _, name := range files {
			// Seeds above 100 KB make mutation too slow; golden tests cover those files.
			if b, err := os.ReadFile(name); err == nil && len(b) <= 100<<10 {
				f.Add(b)
			}
		}
	}
	f.Add(cfbFile(xlsFile(nil, labelRec(0, 0, "x"), mulRKRec(1, 0, 22, 23)), 512))
	f.Add(cfbFile(xlsFile(nil, labelRec(0, 0, "x")), 4096))
	f.Add(cfbFile(xlsFile([][]byte{rec(0xFC, u32s(1, 1), []byte{1, 0, 0}), u16s(0x3C, 5000)}), 512))

	f.Fuzz(func(t *testing.T, data []byte) {
		// The fuzzer has no per-input timeout, so a hang would stall it silently.
		defer time.AfterFunc(10*time.Second, func() {
			pprof.Lookup("goroutine").WriteTo(os.Stderr, 1)
			panic("input takes more than 10s")
		}).Stop()
		wb, err := openReader(bytes.NewReader(data))
		if err != nil {
			return
		}
		for i := 0; i < wb.NumSheets(); i++ {
			s := wb.sheets[i]
			_ = wb.prepareSheet(s)
			for r := 0; r <= int(s.MaxRow); r++ {
				row := s.Row(r)
				for c := row.FirstCol(); c <= row.LastCol(); c++ {
					row.Col(c)
				}
			}
		}
		wb.ReadAllCells(1 << 20)
	})
}
