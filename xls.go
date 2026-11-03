package xls

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/mishramadhav/xls/internal/ole2"
)

// Open opens one xls file. The file is read fully and closed before Open returns.
func Open(file string, charset string) (*WorkBook, error) {
	fi, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer fi.Close()
	return OpenReader(fi, charset)
}

// OpenWithCloser opens one xls file and returns the open file as closer.
// The content is already in memory, so closing it early is safe.
func OpenWithCloser(file string, charset string) (*WorkBook, io.Closer, error) {
	fi, err := os.Open(file)
	if err != nil {
		return nil, nil, err
	}
	wb, err := OpenReader(fi, charset)
	if err != nil {
		fi.Close()
		return nil, nil, err
	}
	return wb, fi, nil
}

// OpenReader opens an xls file from a reader. The charset argument is unused;
// BIFF8 strings are Unicode and BIFF5 strings follow the workbook CODEPAGE.
// It never panics: any panic from malformed input is returned as an error.
func OpenReader(reader io.ReadSeeker, charset string) (wb *WorkBook, err error) {
	defer func() {
		if r := recover(); r != nil {
			wb, err = nil, fmt.Errorf("xls: malformed file: %v", r)
		}
	}()
	return openReader(reader)
}

// openReader is OpenReader without the recover, so fuzzing sees real panics.
func openReader(r io.Reader) (*WorkBook, error) {
	o, err := ole2.Open(r)
	if err != nil {
		return nil, err
	}
	data, err := o.Stream("Workbook", "Book")
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, errors.New("xls: empty workbook stream")
	}
	wb := &WorkBook{Formats: make(map[uint16]*Format), data: data}
	if err := wb.parseGlobals(); err != nil {
		return nil, err
	}
	return wb, nil
}
