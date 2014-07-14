package mail

import (
	"bytes"
	"strconv"
	"strings"

	"github.com/paulrosania/go-charset/charset"
	_ "github.com/paulrosania/go-charset/data"
)

// Returns a copy of this string where each run of whitespace is compressed to
// a single ASCII 32, and where leading and trailing whitespace is removed
// altogether.
func simplify(str string) string {
	if str == "" {
		return ""
	}

	i := 0
	first := 0
	for i < len(str) && first == i {
		c := str[i]
		if c == 9 || c == 10 || c == 13 || c == 32 {
			first++
		}
		i++
	}

	// scan on to find the last nonwhitespace character and detect any
	// sequences of two or more whitespace characters within the
	// string.
	last := first
	spaces := 0
	identity := true
	for identity && i < len(str) {
		c := str[i]
		if c == 9 || c == 10 || c == 13 || c == 32 {
			spaces++
		} else {
			if spaces > 1 {
				identity = false
			}
			spaces = 0
			last = i
		}
		i++
	}

	if identity {
		return str[first : last+1]
	}

	result := make([]rune, 0, len(str))
	i = 0
	spaces = 0
	for i < len(str) {
		c := str[i]
		if c == 9 || c == 10 || c == 13 || c == 32 {
			spaces++
		} else {
			if spaces > 0 && len(result) > 0 {
				result = append(result, ' ')
			}
			spaces = 0
			result = append(result, rune(c))
		}
		i++
	}
	return string(result)
}

// Returns a copy of this string where all letters have been changed to conform
// to typical mail header practice: Letters following digits and other letters
// are lower-cased. Other letters are upper-cased (notably including the very
// first character).
func headerCase(str string) string {
	var buf bytes.Buffer
	i := 0
	u := true

	for i < len(str) {
		c := str[i]
		if u && c >= 'a' && c <= 'z' {
			buf.WriteByte(c - 32)
		} else if !u && c >= 'A' && c <= 'Z' {
			buf.WriteByte(c + 32)
		} else {
			buf.WriteByte(c)
		}

		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			u = false
		} else {
			u = true
		}
		i++
	}
	return buf.String()
}

// Returns a copy of this string where leading and trailing whitespace have
// been removed.
func trim(str string) string {
	return strings.Trim(str, "\t\r\n ")
}

// Returns section \a n of this string, where a section is defined as a run of
// sequences separated by \a s. If \a s is the empty string or \a n is 0,
// section() returns this entire string. If this string contains fewer
// instances of \a s than \a n (ie. section \a n is after the end of the
// string), section returns an empty string.
func section(str, s string, n int) string {
	if s == "" || n == 0 {
		return str
	}

	parts := strings.Split(str, s)
	if n <= len(parts) {
		return parts[n-1]
	}
	return ""
}

var from64 = []uint8{
	64, 99, 99, 99, 99, 99, 99, 99,
	65, 99, 65, 99, 99, 65, 99, 99,
	99, 99, 99, 99, 99, 99, 99, 99,
	99, 99, 99, 99, 99, 99, 99, 99,

	// 32
	99, 99, 99, 99, 99, 99, 99, 99,
	99, 99, 99, 62, 99, 99, 99, 63,
	52, 53, 54, 55, 56, 57, 58, 59,
	60, 61, 99, 99, 99, 64, 99, 99,

	// 64
	99, 0, 1, 2, 3, 4, 5, 6,
	7, 8, 9, 10, 11, 12, 13, 14,
	15, 16, 17, 18, 19, 20, 21, 22,
	23, 24, 25, 99, 99, 99, 99, 99,

	// 96
	99, 26, 27, 28, 29, 30, 31, 32,
	33, 34, 35, 36, 37, 38, 39, 40,
	41, 42, 43, 44, 45, 46, 47, 48,
	49, 50, 51, 99, 99, 99, 99, 99,
}

// Decodes this string using the base-64 algorithm and returns the result.
func de64(s string) string {
	buf := bytes.NewBuffer(make([]byte, 0, len(s)*3/4+20)) // 20 = fudge
	decoded := uint8(0)
	m := 0
	p := 0
	done := false
	for p < len(s) && !done {
		c := s[p]
		if c <= 'z' {
			c = from64[c]
		}
		if c < 64 {
			switch m {
			case 0:
				decoded = c << 2
			case 1:
				decoded += (c & 0xF0) >> 4
				buf.WriteByte(decoded)
				decoded = (c & 15) << 4
			case 2:
				decoded += (c & 0xFC) >> 2
				buf.WriteByte(decoded)
				decoded = (c & 3) << 6
			case 3:
				decoded += c
				buf.WriteByte(decoded)
			}
			m = (m + 1) & 3
		} else if c == 64 {
			done = true
		} else if c == 65 {
			// white space; perfectly normal and may be ignored.
		} else {
			// we're supposed to ignore all other characters. so
			// that's what we do, even though it may not be ideal in
			// all cases... consider that later.
		}
		p++
	}
	return buf.String()
}

// Decodes this string according to the quoted-printable algorithm, and returns
// the result. Errors are overlooked, to cope with all the mail-munging
// brokenware in the great big world.
//
// If \a underscore is true, underscores in the input are translated into
// spaces (as specified in RFC 2047).
func deQP(s string, underscore bool) string {
	i := 0
	buf := bytes.NewBuffer(make([]byte, 0, len(s)))
	for i < len(s) {
		var c byte
		if s[i] != '=' {
			c = s[i]
			i++
			if underscore && c == '_' {
				c = ' '
			}
			buf.WriteByte(c)
		} else {
			// are we looking at = followed by end-of-line?
			var err error
			c = 0
			eol := false
			j := i + 1
			// skip possibly appended whitespace first
			for j < len(s) && (s[j] == ' ' || s[j] == '\t') {
				j++
			}
			// there are two types of soft EOLs:
			if j < len(s) && s[j] == 10 {
				eol = true
				j++
			} else if j < len(s)-1 && s[j] == 13 && s[j+1] == 10 {
				eol = true
				j += 2
			} else if i+2 < len(s) {
				// ... and one common case: a two-digit hex number, not EOL
				n, e := strconv.ParseInt(s[i+1:i+1+2], 16, 8)
				err = e
				c = byte(n)
			}

			// write the proper decoded string and increase i.
			if eol { // ... if it's a soft EOL
				i = j
			} else if err == nil { // ... or if it's a two-digit hex number
				buf.WriteByte(c)
				i += 3
			} else {
				buf.WriteByte(s[i])
				i++
			}
		}
	}
	return buf.String()
}

func decode(s string, enc string) (string, error) {
	buf := bytes.NewBuffer(make([]byte, 0, len(s)))
	cw, err := charset.NewWriter(enc, buf)
	if err != nil {
		return "", err
	}
	_, err = cw.Write([]byte(s))
	return buf.String(), err
}
