package xls

// Ranger is a range of rows.
type Ranger interface {
	FirstRow() uint16
	LastRow() uint16
}

// CellRange is a range of cells over several rows.
type CellRange struct {
	FirstRowB uint16
	LastRowB  uint16
	FristColB uint16
	LastColB  uint16
}

func (c *CellRange) FirstRow() uint16 {
	return c.FirstRowB
}

func (c *CellRange) LastRow() uint16 {
	return c.LastRowB
}

func (c *CellRange) FirstCol() uint16 {
	return c.FristColB
}

func (c *CellRange) LastCol() uint16 {
	return c.LastColB
}

// HyperLink is a hyperlink record. It is listed in WorkSheet.HyperLinks and does
// not replace the text of the cells it covers.
type HyperLink struct {
	CellRange
	Description      string
	TextMark         string
	TargetFrame      string
	Url              string
	ShortedFilePath  string
	ExtendedFilePath string
	IsUrl            bool
}
