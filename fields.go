package mail

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
)

const (
	FromFieldName                    = "From"
	ResentFromFieldName              = "Resent-From"
	SenderFieldName                  = "Sender"
	ResentSenderFieldName            = "Resent-Sender"
	ReturnPathFieldName              = "Return-Path"
	ReplyToFieldName                 = "Reply-To"
	ToFieldName                      = "To"
	CcFieldName                      = "Cc"
	BccFieldName                     = "Bcc"
	ResentToFieldName                = "Resent-To"
	ResentCcFieldName                = "Resent-Cc"
	ResentBccFieldName               = "Resent-Bcc"
	MessageIdFieldName               = "Message-Id"
	ResentMessageIdFieldName         = "Resent-Message-Id"
	InReplyToFieldName               = "In-Reply-To"
	ReferencesFieldName              = "References"
	DateFieldName                    = "Date"
	OrigDateFieldName                = "Orig-Date"
	ResentDateFieldName              = "Resent-Date"
	SubjectFieldName                 = "Subject"
	CommentsFieldName                = "Comments"
	KeywordsFieldName                = "Keywords"
	ContentTypeFieldName             = "Content-Type"
	ContentTransferEncodingFieldName = "Content-Transfer-Encoding"
	ContentDispositionFieldName      = "Content-Disposition"
	ContentDescriptionFieldName      = "Content-Description"
	ContentIdFieldName               = "Content-Id"
	MimeVersionFieldName             = "Mime-Version"
	ReceivedFieldName                = "Received"
	ContentLanguageFieldName         = "Content-Language"
	ContentLocationFieldName         = "Content-Location"
	ContentMd5FieldName              = "Content-Md5"
	ListIdFieldName                  = "List-Id"
	ContentBaseFieldName             = "Content-Base"
	ErrorsToFieldName                = "Errors-To"
)

type Field interface {
	Name() string
	Value() string

	Parse(value string)
	Valid() bool
	SetUnparsedValue(value string)
}

type HeaderField struct {
	name, value   string
	UnparsedValue string
	Error         error
}

func (f *HeaderField) Name() string {
	return f.name
}

func (f *HeaderField) Value() string {
	return f.value
}

// Every HeaderField subclass must define a parse() function that takes a
// string \a s from a message and sets the field value(). This default function
// handles fields that are not specially handled by subclasses using functions
// like parseText().
func (f *HeaderField) Parse(s string) {
	switch f.name {
	case FromFieldName, ResentFromFieldName, SenderFieldName, ReturnPathFieldName,
		ResentSenderFieldName, ToFieldName, CcFieldName, BccFieldName, ReplyToFieldName,
		ResentToFieldName, ResentCcFieldName, ResentBccFieldName, MessageIdFieldName,
		ContentIdFieldName, ResentMessageIdFieldName, ReferencesFieldName, DateFieldName,
		OrigDateFieldName, ResentDateFieldName, ContentTypeFieldName,
		ContentTransferEncodingFieldName, ContentDispositionFieldName,
		ContentLanguageFieldName:
		// These should be handled by their own parse()
	case ContentDescriptionFieldName, SubjectFieldName, CommentsFieldName:
		f.parseText(s)
	case MimeVersionFieldName:
		f.parseMimeVersion(s)
	case ContentLocationFieldName:
		f.parseContentLocation(s)
	case InReplyToFieldName, KeywordsFieldName, ReceivedFieldName, ContentMd5FieldName:
		f.parseOther(s)
	case ContentBaseFieldName:
		f.parseContentBase(s)
	case ErrorsToFieldName:
		f.parseErrorsTo(s)
	default:
		f.parseOther(s)
	}
	log.Printf("Parse: value = %q", f.value)
}

// Parses the *text production from \a s, as modified to include encoded-words
// by RFC 2047. This is used to parse the Subject and Comments fields.
func (f *HeaderField) parseText(s string) {
	h := false

	if !h {
		p := NewParser(s)
		t := p.Text()
		if p.AtEnd() {
			f.value = trim(t)
			h = true
		}
	}

	if !h {
		p := NewParser(simplify(s))
		t := p.Text()
		if p.AtEnd() {
			f.value = t
			h = true
		}
	}

	if (!h && strings.Contains(s, "=?") && strings.Contains(s, "?=")) ||
		(strings.Contains(f.value, "=?") && strings.Contains(f.value, "?=")) {
		// common: Subject: =?ISO-8859-1?q?foo bar baz?=
		// unusual, but seen: Subject: =?ISO-8859-1?q?foo bar?= baz
		p1 := NewParser(simplify(s))
		var tmp bytes.Buffer
		inWord := false
		for !p1.AtEnd() {
			if p1.Present("=?") {
				inWord = true
				tmp.WriteString(" =?")
			} else if p1.Present("?=") {
				inWord = false
				tmp.WriteString("?= ")
			} else if p1.Whitespace() == "" {
				tmp.WriteByte(p1.NextChar())
				p1.Step(1)
			} else {
				if inWord {
					tmp.WriteByte('_')
				} else {
					tmp.WriteByte(' ')
				}
			}
		}
		p2 := NewParser(tmp.String())
		t := simplify(p2.Text())
		if p2.AtEnd() && !strings.Contains(t, "?=") {
			f.value = t
			h = true
		}
	}

	if !h {
		f.Error = errors.New("Error parsing text")
	}
}

// Parses the Mime-Version field from \a s and resolutely ignores all problems
// seen.
//
// Only version 1.0 is legal. Since vast numbers of spammers send other version
// numbers, we replace other version numbers with 1.0 and a comment. Bayesian
// analysis tools will probably find the comment to be a sure spam sign.
func (f *HeaderField) parseMimeVersion(s string) {
	p := NewParser(s)
	p.Comment()
	v := p.DotAtom()
	p.Comment()
	c, err := decode(p.lc, "us-ascii")
	if err != nil || strings.ContainsAny(c, "()\\") {
		c = ""
	}
	if v != "1.0" || !p.AtEnd() {
		c = "Note: Original mime-version had syntax problems"
	}
	result := "1.0"
	if c != "" {
		result += "(" + c + ")"
	}
	f.value = result
}

// Parses the Content-Location header field in \a s and records the first
// problem found.
func (f *HeaderField) parseContentLocation(s string) {
	p := NewParser(unquote(trim(s), '"', '\''))

	p.Whitespace()
	e := p.Pos()
	ok := true
	var buf bytes.Buffer
	for ok {
		ok = true
		c := p.NextChar()
		p.Step(1)
		if c == '%' {
			hex := make([]byte, 2)
			hex[0] = p.NextChar()
			p.Step(1)
			hex[1] = p.NextChar()
			p.Step(1)
			i, err := strconv.ParseInt(string(hex), 16, 8)
			if err != nil {
				ok = false
			}
			c = byte(i)
		}

		if (c >= 'a' && c <= 'z') || // alpha
			(c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || // letter
			c == '$' || c == '-' || // safe
			c == '_' || c == '.' ||
			c == '+' ||
			c == '!' || c == '*' || // extra
			c == '\'' || c == '(' ||
			c == ')' || c == ',' {
			// RFC 1738 unreserved
			buf.WriteByte(c)
		} else if c == ';' || c == '/' || c == '?' ||
			c == ':' || c == '@' || c == '&' ||
			c == '=' {
			// RFC 1738 reserved
			buf.WriteByte(c)
		} else if c == '%' || c >= 127 {
			// RFC 1738 escape
			hex := strconv.FormatInt(int64(c), 16)
			buf.WriteByte('%')
			if len(hex) < 2 {
				buf.WriteByte('0')
			}
			buf.WriteString(hex)
		} else if c == ' ' {
			// seen in real life, sent by buggy programs
			buf.WriteString("%20")
		} else if c == '\r' || c == '\n' {
			// and another kind of bug, except that in this case, is there a
			// right kind of way? let's not flame programs which do this.
			p.Whitespace()
		} else {
			ok = false
		}
		if ok {
			e = p.Pos()
		}
	}
	p.Whitespace()

	v, err := decode(buf.String(), "us-ascii")
	f.value = v
	if !p.AtEnd() {
		f.Error = fmt.Errorf("Junk at position %d: %s", e, s[e:])
	} else if err != nil {
		f.Error = err
	}
}

// Tries to parses any (otherwise uncovered and presumably unstructured) field
// in \a s, and records an error if it contains NULs or 8-bit characters.
func (f *HeaderField) parseOther(s string) {
	v, err := decode(s, "us-ascii")
	if err != nil {
		f.Error = err
	}
	f.value = v
}

// Parses the Content-Base header field in \a s and records the first problem
// found. Somewhat overflexibly assumes that if there is a colon, the URL is
// absolute, so it accepts -:/asr as a valid URL.
func (f *HeaderField) parseContentBase(s string) {
	f.parseContentLocation(s)
	if !f.Valid() {
		return
	}
	if !strings.Contains(f.value, ":") {
		f.Error = errors.New("URL has no scheme")
	}
}

// Parses Errors-To field \a s. Stores localpart@domain if it looks like a
// single address (and reasonably error-free) and an empty value if there's any
// doubt what to store.
func (f *HeaderField) parseErrorsTo(s string) {
}

// Returns true if this header field is valid (or unparsed, as is the case for
// all unknown fields), and false if an error was detected during parsing.
func (f *HeaderField) Valid() bool {
	return f.Error == nil
}

func (f *HeaderField) SetUnparsedValue(value string) {
	f.UnparsedValue = value
}

type AddressField struct {
	HeaderField
}

func NewAddressField(name string) *AddressField {
	hf := HeaderField{name: name}
	return &AddressField{hf}
}

func (f *AddressField) Parse(value string) {
}

type DateField struct {
	HeaderField
}

func NewDateField() *DateField {
	hf := HeaderField{name: DateFieldName}
	return &DateField{hf}
}

func (f *DateField) Parse(value string) {
}

type ContentType struct {
	HeaderField
}

func NewContentType() *ContentType {
	hf := HeaderField{name: ContentTypeFieldName}
	return &ContentType{hf}
}

func (f *ContentType) Parse(value string) {
}

type ContentTransferEncoding struct {
	HeaderField
}

func NewContentTransferEncoding() *ContentTransferEncoding {
	hf := HeaderField{name: ContentTransferEncodingFieldName}
	return &ContentTransferEncoding{hf}
}

func (f *ContentTransferEncoding) Parse(value string) {
}

type ContentDisposition struct {
	HeaderField
}

func NewContentDisposition() *ContentDisposition {
	hf := HeaderField{name: ContentDispositionFieldName}
	return &ContentDisposition{hf}
}

func (f *ContentDisposition) Parse(value string) {
}

type ContentLanguage struct {
	HeaderField
}

func NewContentLanguage() *ContentLanguage {
	hf := HeaderField{name: ContentLanguageFieldName}
	return &ContentLanguage{hf}
}

func (f *ContentLanguage) Parse(value string) {
}

func NewHeaderFieldNamed(name string) Field {
	n := headerCase(name)

	var hf Field
	switch n {
	case InReplyToFieldName, SubjectFieldName, CommentsFieldName, KeywordsFieldName,
		ContentDescriptionFieldName, MimeVersionFieldName, ReceivedFieldName,
		ContentLocationFieldName, ContentMd5FieldName, ListIdFieldName:
		hf = &HeaderField{name: n}
	case FromFieldName, ResentFromFieldName, SenderFieldName, ResentSenderFieldName,
		ReturnPathFieldName, ReplyToFieldName, ToFieldName, CcFieldName, BccFieldName,
		ResentToFieldName, ResentCcFieldName, ResentBccFieldName, MessageIdFieldName,
		ContentIdFieldName, ResentMessageIdFieldName, ReferencesFieldName:
		hf = NewAddressField(n)
	case DateFieldName, OrigDateFieldName, ResentDateFieldName:
		hf = NewDateField()
	case ContentTypeFieldName:
		hf = NewContentType()
	case ContentTransferEncodingFieldName:
		hf = NewContentTransferEncoding()
	case ContentDispositionFieldName:
		hf = NewContentDisposition()
	case ContentLanguageFieldName:
		hf = NewContentLanguage()
	default:
		hf = &HeaderField{name: n}
	}

	return hf
}

func NewHeaderField(name, value string) Field {
	hf := NewHeaderFieldNamed(name)
	hf.Parse(value)
	if hf.Valid() {
		return hf
	}

	i := 0
	for value[i] == ':' || value[i] == ' ' {
		i++
	}
	suf := NewHeaderFieldNamed(name)
	suf.Parse(value[i:])
	if suf.Valid() {
		return suf
	}
	hf.SetUnparsedValue(value)
	return hf
}
