package xls

import (
	"math"
	"strings"
	"time"
	"unicode"
)

// timeFromExcelTime converts a serial date. In the 1900 system Excel counts a
// non-existent 1900-02-29 as serial 60, so serials below 61 are one day off
// the later base date; serial 60 maps to 1900-03-01.
func timeFromExcelTime(excelTime float64, date1904 bool) time.Time {
	days, frac := math.Modf(excelTime)
	base := time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)
	switch {
	case date1904:
		base = time.Date(1904, 1, 1, 0, 0, 0, 0, time.UTC)
	case days < 61:
		base = time.Date(1899, 12, 31, 0, 0, 0, 0, time.UTC)
	}
	ms := time.Duration(math.Round(frac*24*3600*1000)) * time.Millisecond
	return base.AddDate(0, 0, int(days)).Add(ms)
}

// goLayout converts an Excel date/time format code to a Go time layout. It
// returns "" when the code has text, a fraction or a letter it cannot map.
func goLayout(f string) string {
	type tok struct {
		code string // run of one letter, lower case, or literal text
		lit  bool
	}
	var toks []tok
	var ampm bool
	rs := []rune(f)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		switch {
		case r == '[':
			for i < len(rs) && rs[i] != ']' {
				i++
			}
		case r == '"' || r == '\\' || r == '_' || r == '*':
			return ""
		case strings.ContainsRune("dmyhs", unicode.ToLower(r)):
			j := i
			for j+1 < len(rs) && unicode.ToLower(rs[j+1]) == unicode.ToLower(r) {
				j++
			}
			toks = append(toks, tok{code: strings.ToLower(string(rs[i : j+1]))})
			i = j
		case (r == 'A' || r == 'a') && len(rs) >= i+5 && strings.EqualFold(string(rs[i:i+5]), "am/pm"):
			ampm = true
			toks = append(toks, tok{code: "pm"})
			i += 4
		case strings.ContainsRune("-/:, .", r):
			toks = append(toks, tok{code: string(r), lit: true})
		default:
			return ""
		}
	}
	var sb strings.Builder
	for i, t := range toks {
		if t.lit {
			sb.WriteString(t.code)
			continue
		}
		prev, next := "", ""
		for k := i - 1; k >= 0; k-- {
			if !toks[k].lit {
				prev = toks[k].code
				break
			}
		}
		for k := i + 1; k < len(toks); k++ {
			if !toks[k].lit {
				next = toks[k].code
				break
			}
		}
		switch t.code {
		case "yyyy":
			sb.WriteString("2006")
		case "yy", "y":
			sb.WriteString("06")
		case "mmmm":
			sb.WriteString("January")
		case "mmm":
			sb.WriteString("Jan")
		case "mm", "m":
			minute := strings.HasPrefix(prev, "h") || strings.HasPrefix(next, "s")
			switch {
			case minute && t.code == "mm":
				sb.WriteString("04")
			case minute:
				sb.WriteString("4")
			case t.code == "mm":
				sb.WriteString("01")
			default:
				sb.WriteString("1")
			}
		case "dddd":
			sb.WriteString("Monday")
		case "ddd":
			sb.WriteString("Mon")
		case "dd":
			sb.WriteString("02")
		case "d":
			sb.WriteString("2")
		case "hh", "h":
			switch {
			case ampm && t.code == "hh":
				sb.WriteString("03")
			case ampm:
				sb.WriteString("3")
			default:
				sb.WriteString("15")
			}
		case "ss":
			sb.WriteString("05")
		case "s":
			sb.WriteString("5")
		case "pm":
			sb.WriteString("PM")
		default:
			return ""
		}
	}
	return sb.String()
}

// formatDate renders t with an Excel format code. A code that goLayout cannot
// map gives ISO 8601 text, with the time only when t has one.
func formatDate(t time.Time, code string) string {
	if l := goLayout(code); l != "" {
		return t.Format(l)
	}
	if t.Hour() == 0 && t.Minute() == 0 && t.Second() == 0 {
		return t.Format("2006-01-02")
	}
	return t.Format("2006-01-02 15:04:05")
}
