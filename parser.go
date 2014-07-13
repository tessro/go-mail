package mail

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"strings"

	"code.google.com/p/go-charset/charset"
	_ "code.google.com/p/go-charset/data"
)

type parserState struct {
	at  int
	err error

	next *parserState
	mark int
}

func newParserState(other *parserState) *parserState {
	ps := &parserState{mark: 1}
	if other != nil {
		ps.at = other.at
		ps.next = other
		ps.mark = other.mark + 1
	}
	return ps
}

type Parser struct {
	*parserState
	str string

	mime bool
	lc   string
}

func NewParser(s string) *Parser {
	st := newParserState(nil)
	return &Parser{str: s, parserState: st}
}

// Moves pos() to the first nonwhitespace character after the current point.
// If pos() points to nonwhitespace already, it is not moved.
func (p *Parser) Whitespace() string {
	var out bytes.Buffer

	c := p.NextChar()
	for c == ' ' || c == 9 || c == 10 || c == 13 || c == 160 {
		out.WriteByte(c)
		p.Step(1)
		c = p.NextChar()
	}

	return out.String()
}

type EncodedTextType int

const (
	EncodedText EncodedTextType = iota
	EncodedComment
	EncodedPhrase
)

type EncodingType int

const (
	QPEncoding EncodingType = iota
	Base64Encoding
)

// Steps past a MIME encoded-word (as defined in RFC 2047) and returns its
// decoded unicode representation, or an empty string if the cursor does not
// point to a valid encoded-word. The caller is responsible for checking that
// the encoded-word is separated from neighbouring tokens by whitespace.
//
// The characters permitted in the encoded-text are adjusted based on \a type,
// which may be Text (by default), Comment, or Phrase.
func (p *Parser) encodedWord(t EncodedTextType) string {
	// encoded-word = "=?" charset '?' encoding '?' encoded-text "?="

	m := p.mark()
	p.require("=?")
	if !p.Valid() {
		p.restore(m)
		return ""
	}

	var csBuf bytes.Buffer
	c := p.NextChar()
	for c > 32 && c < 128 &&
		c != '(' && c != ')' && c != '<' && c != '>' &&
		c != '@' && c != ',' && c != ';' && c != ':' &&
		c != '[' && c != ']' && c != '?' && c != '=' &&
		c != '\\' && c != '"' && c != '/' && c != '.' {
		csBuf.WriteByte(c)
		p.Step(1)
		c = p.NextChar()
	}
	cs := csBuf.String()
	if strings.ContainsRune(cs, '*') {
		// XXX: What should we do with the language information?
		cs = section(cs, "*", 1)
	}

	p.require("?")

	encoding := QPEncoding
	if p.Present("q") {
		encoding = QPEncoding
	} else if p.Present("b") {
		encoding = Base64Encoding
	} else {
		p.err = fmt.Errorf("Unknown encoding: %s", p.NextChar())
	}

	p.require("?")

	var buf bytes.Buffer
	c = p.NextChar()
	if encoding == Base64Encoding {
		for (c >= '0' && c <= '9') ||
			(c >= 'a' && c <= 'z') ||
			(c >= 'A' && c <= 'Z') ||
			c == '+' || c == '/' || c == '=' {
			buf.WriteByte(c)
			p.Step(1)
			c = p.NextChar()
		}
	} else {
		for c > 32 && c < 128 && c != '?' &&
			(t != EncodedComment ||
				(c != '(' && c != ')' && c != '\\')) &&
			(t != EncodedPhrase ||
				(c >= '0' && c <= '9') ||
				(c >= 'a' && c <= 'z') ||
				(c >= 'A' && c <= 'Z') ||
				(c == '!' || c == '*' || c == '-' ||
					c == '/' || c == '=' || c == '_' ||
					c == '\'')) {
			buf.WriteByte(c)
			p.Step(1)
			c = p.NextChar()
		}
	}

	p.require("?=")

	if !p.Valid() {
		p.restore(m)
		return ""
	}

	text := buf.String()
	if encoding == QPEncoding {
		text = deQP(text, false)
	} else {
		text = de64(text)
	}
	r := strings.NewReader(text)
	cr, err := charset.NewReader(cs, r)
	if err != nil {
		// XXX: Should we treat unknown charsets as us-ascii?
		p.err = fmt.Errorf("Unknown character set: %s", cs)
		p.restore(m)
		return ""
	}
	bs, err := ioutil.ReadAll(cr)
	if err != nil {
		p.err = err
		p.restore(m)
		return ""
	}
	result := string(bs)

	if strings.ContainsAny(result, "\r\n") {
		result = simplify(result) // defend against =?ascii?q?x=0aEvil:_nasty?=
	}

	if strings.IndexByte(result, 8) >= 0 { // we interpret literal DEL. fsck.
		i := 0
		for i >= 0 {
			i = strings.IndexByte(result[i:], 8)
			if i >= 0 {
				s := result[i+1:]
				if i > 1 {
					result = result[:i-1] + s
				} else {
					result = s
				}
			}
		}
	}

	return result
}

// Steps past a sequence of adjacent encoded-words with whitespace in between
// and returns the decoded representation. \a t passed through to
// encodedWord().
//
// Leading and trailing whitespace is trimmed, internal whitespace is kept as
// is.
func (p *Parser) encodedWords(t EncodedTextType) string {
	var out bytes.Buffer
	end := false
	var m int
	for !end {
		m = p.mark()
		p.Whitespace()
		n := p.pos()
		s := p.encodedWord(t)
		if n == p.pos() {
			end = true
		} else {
			out.WriteString(s)
		}
	}

	p.restore(m)
	return trim(out.String())
}

func (p *Parser) Text() string {
	var out bytes.Buffer

	space := p.Whitespace()
	var word string
	progress := true
	for progress {
		m := p.mark()
		start := p.pos()

		encodedWord := false

		if p.Present("=?") {
			p.restore(m)
			encodedWord = true
			word = p.encodedWords(EncodedText)
			if p.pos() == start {
				encodedWord = false
			}
		}

		if !encodedWord {
			var buf bytes.Buffer
			c := p.NextChar()
			for !p.AtEnd() && c < 128 && c != ' ' && c != 9 && c != 10 && c != 13 {
				buf.WriteByte(c)
				p.Step(1)
				c = p.NextChar()
			}
			word = buf.String()
		}

		if p.pos() == start {
			progress = false
		} else {
			out.WriteString(space)
			out.WriteString(word)

			space = p.Whitespace()
			if strings.ContainsAny(space, "\r\n") {
				space = " "
			}
		}
	}

	if len(space) != 0 {
		out.WriteString(space)
	}

	return out.String()
}

// Returns the current (0-indexed) position of the cursor in the input() string
// without changing anything.
func (p *Parser) pos() int {
	return p.at
}

// Returns the next character at the cursor without changing the cursor
// position. Returns 0 if there isn't a character available (e.g. when the
// cursor is past the end of the input string).
func (p *Parser) NextChar() uint8 {
	if p.at >= len(p.str) {
		return 0
	} else {
		return p.str[p.at]
	}
}

// Advances the cursor past n characters of the input.
func (p *Parser) Step(n int) {
	p.at += n
}

// Checks whether the next characters in the input match s. If so, Present()
// steps past the matching characters and returns true. If not, it returns
// false without advancing the cursor. The match is case insensitive.
func (p *Parser) Present(s string) bool {
	if s == "" {
		return true
	}
	if p.at+len(s) > len(p.str) {
		return false
	}

	l := strings.ToLower(p.str[p.at : p.at+len(s)])
	s = strings.ToLower(s)
	log.Printf("wanted %q got %q (%q)", s, l, p.following())
	if l != s {
		return false
	}

	p.Step(len(s))
	return true
}

// Requires that the next characters in the input match \a s (case
// insensitively), and steps past the matching characters. If \a s is not
// present(), it is considered an error().
func (p *Parser) require(s string) {
	if !p.Present(s) {
		p.err = fmt.Errorf("Expected: %q, got: %s", s, p.following())
	}
}

// Returns a string of no more than 15 characters containing the first unparsed
// bits of input. Meant for use in error messages.
func (p *Parser) following() string {
	if p.at >= len(p.str) {
		return ""
	}
	f := p.str[p.at:]
	if len(f) > 15 {
		f = f[:15]
	}
	return simplify(f)
}

// Returns true if we have parsed the entire input string, and false otherwise.
func (p *Parser) AtEnd() bool {
	return p.at >= len(p.str)
}

// Saves the current cursor position and error state of the parser and returns
// an identifier of the current mark. The companion function restore() restores
// the last or a specified mark. The returned mark is never 0.
func (p *Parser) mark() int {
	p.parserState = newParserState(p.parserState)
	return p.next.mark
}

// Restores the last mark()ed cursor position and error state of this parser
// object.
func (p *Parser) restore(m int) {
	c := p.parserState
	for c != nil && c.mark != m && c.next != nil {
		c = c.next
	}
	if c != nil && c.mark == m {
		p.parserState = c
	}
}

func (p *Parser) Valid() bool {
	return p.err == nil
}
