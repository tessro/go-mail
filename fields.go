package mail

import (
	"bytes"
	"errors"
	"log"
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
	Parse(value string)
	Valid() bool
	SetUnparsedValue(value string)
}

type HeaderField struct {
	Name, Value   string
	UnparsedValue string
	Error         error
}

func (f *HeaderField) Parse(s string) {
	switch f.Name {
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
	log.Printf("Parse: value = %q", f.Value)
}

func (f *HeaderField) parseText(s string) {
	h := false

	if !h {
		p := NewParser(s)
		t := p.Text()
		if p.AtEnd() {
			f.Value = trim(t)
			h = true
		}
	}

	if !h {
		p := NewParser(simplify(s))
		t := p.Text()
		if p.AtEnd() {
			f.Value = t
			h = true
		}
	}

	if (!h && strings.Contains(s, "=?") && strings.Contains(s, "?=")) ||
		(strings.Contains(f.Value, "=?") && strings.Contains(f.Value, "?=")) {
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
			f.Value = t
			h = true
		}
	}

	if !h {
		f.Error = errors.New("Error parsing text")
	}
}

func (f *HeaderField) parseMimeVersion(s string) {
}

func (f *HeaderField) parseContentLocation(s string) {
}

func (f *HeaderField) parseOther(s string) {
}

func (f *HeaderField) parseContentBase(s string) {
}

func (f *HeaderField) parseErrorsTo(s string) {
}

func (f *HeaderField) Valid() bool {
	return f.Error == nil
}

func (f *HeaderField) SetUnparsedValue(value string) {
	f.UnparsedValue = value
}

type AddressField struct {
	Name string
}

func (f *AddressField) Parse(value string) {
}

func (f *AddressField) Valid() bool {
	return true
}

func (f *AddressField) SetUnparsedValue(value string) {
}

type DateField struct {
	Name string
}

func (f *DateField) Parse(value string) {
}

func (f *DateField) Valid() bool {
	return true
}

func (f *DateField) SetUnparsedValue(value string) {
}

type ContentType struct {
}

func (f *ContentType) Parse(value string) {
}

func (f *ContentType) Valid() bool {
	return true
}

func (f *ContentType) SetUnparsedValue(value string) {
}

type ContentTransferEncoding struct {
}

func (f *ContentTransferEncoding) Parse(value string) {
}

func (f *ContentTransferEncoding) Valid() bool {
	return true
}

func (f *ContentTransferEncoding) SetUnparsedValue(value string) {
}

type ContentDisposition struct {
}

func (f *ContentDisposition) Parse(value string) {
}

func (f *ContentDisposition) Valid() bool {
	return true
}

func (f *ContentDisposition) SetUnparsedValue(value string) {
}

type ContentLanguage struct {
}

func (f *ContentLanguage) Parse(value string) {
}

func (f *ContentLanguage) Valid() bool {
	return true
}

func (f *ContentLanguage) SetUnparsedValue(value string) {
}

func NewHeaderFieldNamed(name string) Field {
	n := headerCase(name)

	var hf Field
	switch n {
	case InReplyToFieldName, SubjectFieldName, CommentsFieldName, KeywordsFieldName,
		ContentDescriptionFieldName, MimeVersionFieldName, ReceivedFieldName,
		ContentLocationFieldName, ContentMd5FieldName, ListIdFieldName:
		hf = &HeaderField{Name: n}
	case FromFieldName, ResentFromFieldName, SenderFieldName, ResentSenderFieldName,
		ReturnPathFieldName, ReplyToFieldName, ToFieldName, CcFieldName, BccFieldName,
		ResentToFieldName, ResentCcFieldName, ResentBccFieldName, MessageIdFieldName,
		ContentIdFieldName, ResentMessageIdFieldName, ReferencesFieldName:
		hf = &AddressField{Name: n}
	case DateFieldName, OrigDateFieldName, ResentDateFieldName:
		hf = &DateField{Name: n}
	case ContentTypeFieldName:
		hf = &ContentType{}
	case ContentTransferEncodingFieldName:
		hf = &ContentTransferEncoding{}
	case ContentDispositionFieldName:
		hf = &ContentDisposition{}
	case ContentLanguageFieldName:
		hf = &ContentLanguage{}
	default:
		hf = &HeaderField{Name: n}
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
