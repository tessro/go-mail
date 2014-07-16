package mail

import (
	"bytes"
	"strconv"
	"strings"

	"github.com/paulrosania/go-charset/charset"
	_ "github.com/paulrosania/go-charset/data"
)

type BoringType int

const (
	TotallyBoring BoringType = iota
	IMAPBoring
	MIMEBoring
)

// Returns true is the string is quoted with \a c (default '"') as quote
// character and \a q (default '\') as escape character. \a c and \a q may be
// the same.
func isQuoted(str string, c, q byte) bool {
	if len(str) < 2 || str[0] != c || str[len(str)-1] != c {
		return false
	}
	// skip past double escapes
	i := len(str) - 2
	for i > 1 && str[i] == q && str[i-1] == q {
		i -= 2
	}
	// empty string left?
	if i == 0 {
		return true
	}
	// trailing quote escaped?
	if str[i] == q {
		return false
	}
	return true
}

// Returns the unquoted representation of the string if it isQuoted() and the
// string itself else.
//
// \a c at the start and end are removed; any occurence of \a c within the
// string is left alone; an occurence of \a q followed by \a c is converted
// into just \a c.
func unquote(str string, c, q byte) string {
	if !isQuoted(str, c, q) {
		return str
	}
	buf := bytes.NewBuffer(make([]byte, 0, len(str)))
	i := 1
	for i < len(str)-1 {
		if str[i] == q {
			i++
		}
		buf.WriteByte(str[i])
		i++
	}
	return buf.String()
}

// Returns a version of this string quited with \a c, and where any occurences
// of \a c or \a q are escaped with \a q.
func quote(str string, c, q byte) string {
	buf := bytes.NewBuffer(make([]byte, 0, len(str)+2))
	buf.WriteByte(c)
	i := 0
	for i < len(str) {
		if str[i] == c || str[i] == q {
			buf.WriteByte(q)
		}
		buf.WriteByte(str[i])
		i++
	}
	buf.WriteByte(c)
	return buf.String()
}

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

// Do RFC 2047 decoding of \a s, totally ignoring what the encoded-text in \a s
// contains.
//
// Depending on circumstances, the encoded-text may contain different sets of
// characters. Moreover, not every 2047 encoder obeys the rules. This function
// checks nothing, it just decodes.
func de2047(s string) string {
	out := ""
	if !strings.HasPrefix(s, "=?") || !strings.HasSuffix(s, "?=") {
		return out
	}
	cs := 2
	ce := strings.IndexByte(s[2:], '*')
	if ce >= 0 {
		ce += 2
	}
	es := strings.IndexByte(s[2:], '?') + 1
	if es >= 1 { // 0 == not found
		es += 2
	}
	if es < cs {
		return out
	}
	if ce < cs {
		ce = es
	}
	if ce >= es {
		ce = es - 1
	}
	if s[es+1] != '?' {
		return out
	}

	encoded := s[es+2 : len(s)-2]
	decoded := ""

	switch s[es] {
	case 'Q', 'q':
		decoded = deQP(encoded, true)
	case 'B', 'b':
		decoded = de64(encoded)
	default:
		return out
	}

	enc := s[cs:ce]
	buf := bytes.NewBuffer(make([]byte, 0, len(encoded)))
	cw, err := charset.NewWriter(enc, buf)
	if err != nil {
		// if we didn't recognise the codec, we'll assume that it's
		// ASCII if that would work and otherwise refuse to decode.
		_, err = decode(decoded, "us-ascii")
		if err != nil {
			return out
		}
		cw, err = charset.NewWriter("us-ascii", buf)
		if err != nil {
			panic(err)
		}
	}
	cw.Write([]byte(s)) // FIXME: Ignores errors
	return buf.String()
}

// This static function returns the RFC 2047-encoded version of \a s.
func encodePhrase(s string) string {
	buf := bytes.NewBuffer(make([]byte, 0, len(s)))
	words := strings.Split(simplify(s), " ")

	for i := 0; i < len(words); i++ {
		w := words[i]

		if i > 0 {
			buf.WriteByte(' ')
		}

		if isAscii(w) && isBoring(ascii(w), TotallyBoring) {
			buf.WriteString(ascii(w))
		} else {
			for i < len(words) && !(isAscii(words[i]) && isBoring(ascii(words[i]), TotallyBoring)) {
				w += " " + words[i]
				i++
			}
			buf.WriteString(encodeWord(words[i]))
		}
	}

	return buf.String()
}

// This static function returns an RFC 2047 encoded-word representing \a w.
func encodeWord(w string) string {
	if w == "" {
		return ""
	}

	return w
	/*
		// FIXME: encode properly
		//Codec * c = Codec::byString( w );
		//EString cw( c->fromUnicode( w ) );
		cw := w

		buf := bytes.NewBuffer(make([]byte, 0, len(w)))
		buf.WriteString("=?")
		buf.WriteString(c.name())
		buf.WriteString("?")
		t := buf.String()
		qp := eQP(cw, true)
		b64 := e64(cw)
		if len(qp) <= len(b64)+3 &&
			len(t)+len(qp) <= 73 {
			buf.WriteString("q?")
			buf.WriteString(qp)
			buf.WriteString("?=")
			t += buf.String() // FIXME: verify append is correct here, first half of buffer should already be in `t`
		} else {
			prefix := t + "b?"
			t = ""
			for b64 != "" {
				allowed := 73 - len(prefix)
				allowed = 4 * (allowed / 4)
				word := prefix
				word += b64[:allowed]
				word += "?="
				b64 = b64[allowed:]
				t += word
				if b64 != "" {
					t += " "
				}
			}
		}

		return t
	*/
}

// Returns true if this string contains only tab, cr, lf and printable ASCII
// characters, and false if it contains one or more other characters.
func isAscii(s string) bool {
	if s == "" {
		return true
	}
	i := 0
	for i < len(s) {
		if s[i] >= 128 || (s[i] < 32 && s[i] != 9 && s[i] != 10 && s[i] != 13) {
			return false
		}
		i++
	}
	return true
}

// Returns a copy of this string in 7-bit ASCII. Any characters that aren't
// printable ascii are changed into '?'. (Is '?' the right choice?)
//
// This looks like AsciiCodec::fromUnicode(), but is semantically different.
// This function is for logging and debugging and may leave out a different set
// of characters than does AsciiCodec::fromUnicode().
func ascii(s string) string {
	buf := bytes.NewBuffer(make([]byte, 0, len(s)))
	i := 0
	for i < len(s) {
		if s[i] >= ' ' && s[i] < 127 {
			buf.WriteByte(s[i])
		} else {
			buf.WriteByte('?')
		}
		i++
	}
	return buf.String()
}

// Returns true if this string is really boring, and false if it's empty or
// contains at least one character that may warrant quoting in some context. So
// far RFC 822 atoms, 2822 atoms, IMAP atoms and MIME tokens are considered.
//
// This function considers the intersection of those character classes to be
// the Totally boring subset. If \a b is not its default value, it may include
// other characters.
func isBoring(s string, b BoringType) bool {
	if s == "" {
		return false // empty strings aren't boring - they may need quoting
	}
	i := 0
	exciting := false
	for i < len(s) && !exciting {
		switch s[i] {
		case 'A', 'B', 'C', 'D', 'E', 'F', 'G', 'H', 'I', 'J', 'K', 'L', 'M', 'N',
			'O', 'P', 'Q', 'R', 'S', 'T', 'U', 'V', 'W', 'X', 'Y', 'Z',
			'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i', 'j', 'k', 'l', 'm', 'n',
			'o', 'p', 'q', 'r', 's', 't', 'u', 'v', 'w', 'x', 'y', 'z',
			'0', '1', '2', '3', '4', '5', '6', '7', '8', '9', '!', '#', '$', '&', '+', '-':
			// boring
		case '.':
			if b != MIMEBoring {
				exciting = true
			}
		default:
			exciting = true
		}
		i++
	}
	// if we saw an exciting character...
	if exciting {
		return false
	}
	return true
}

// Returns a copy of this string wrapped so that each line contains at most \a
// linelength characters. The first line is prefixed by \a firstPrefix,
// subsequent lines by \a otherPrefix. If \a spaceAtEOL is true, all lines
// except the last end with a space.
//
// The prefixes are counted towards line length, but the optional trailing
// space is not.
//
// Only space (ASCII 32) is a line-break opportunity. If there are multiple
// spaces where a line is broken, all the spaces are replaced by a single CRLF.
// Linefeeds added use CRLF.
func wrap(s string, linelength int, firstPrefix, otherPrefix string, spaceAtEOL bool) string {
	buf := bytes.NewBuffer(make([]byte, 0, len(s)))
	buf.WriteString(firstPrefix)

	// move is where we keep the text that has to be moved to the next
	// line. it too should be modifiable() all the time.
	var move bytes.Buffer
	i := 0
	linestart := 0
	space := 0
	for i < len(s) {
		c := s[i]
		if c == ' ' {
			space = buf.Len()
		} else if c == '\n' {
			linestart = buf.Len() + 1
		}
		buf.WriteByte(c)
		i++
		// add a soft linebreak?
		if buf.Len() > linestart+linelength && space > linestart {
			for space > 0 && buf.String()[space-1] == ' ' {
				space--
			}
			linestart = space + 1
			for linestart < buf.Len() && buf.String()[linestart] == ' ' {
				linestart++
			}
			move.Truncate(0)
			if buf.Len() > linestart {
				move.WriteString(buf.String()[linestart:])
			}
			if spaceAtEOL {
				buf.Truncate(space + 1)
			} else {
				buf.Truncate(space)
			}
			buf.WriteString("\r\n")
			buf.WriteString(otherPrefix)
			buf.WriteString(move.String())
		}
	}
	return buf.String()
}
