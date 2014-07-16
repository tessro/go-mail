package mail

import (
	"bytes"
	"errors"
	"fmt"
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
	ap := NewAddressParser(s)

	if ap.firstError != nil || len(ap.addresses) != 1 {
		return
	}

	a := ap.addresses[0]
	if a.t != NormalAddressType {
		return
	}

	v, err := decode(a.lpdomain(), "us-ascii")
	f.value = v
	f.Error = err
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
	Addresses []Address
}

func NewAddressField(name string) *AddressField {
	hf := HeaderField{name: name}
	return &AddressField{HeaderField: hf}
}

// Generates the RFC 822 representation of the field, based on the addresses().
// If \a avoidUTf8 is true, rfc822() will be lossy rather than include any
// UTF-8.
func (f *AddressField) rfc822(avoidUtf8 bool) string {
	s := ""

	name := f.Name()
	if name == ReturnPathFieldName {
		if len(f.Addresses) == 0 {
		} else if f.Addresses[0].t == BounceAddressType {
			s = "<>"
		} else if f.Addresses[0].t == NormalAddressType {
			s = "<" + f.Addresses[0].lpdomain() + ">"
		}
	} else if name == MessageIdFieldName ||
		name == ResentMessageIdFieldName ||
		name == ContentIdFieldName ||
		name == ReferencesFieldName && len(f.Addresses) == 0 {
		if len(f.Addresses) > 0 {
			s = "<" + f.Addresses[0].toString(false) + ">"
		} else {
			s = f.Name() + ": " + ascii(f.Value())
			s = wrap(simplify(s), 78, "", " ", false)
			p := len(f.Name()) + 1
			for p < len(s) &&
				(s[p] == ' ' || s[p] == '\r' || s[p] == '\n') {
				p++
			}
			s = s[p:]
		}
	} else if name == FromFieldName ||
		name == ResentFromFieldName ||
		name == SenderFieldName ||
		name == ResentSenderFieldName ||
		name == ReturnPathFieldName ||
		name == ReplyToFieldName ||
		name == ToFieldName || name == CcFieldName || name == BccFieldName ||
		name == ResentToFieldName || name == ResentCcFieldName || name == ResentBccFieldName ||
		name == ReferencesFieldName {
		first := true
		wsep := ""
		lsep := ""
		c := len(f.Name()) + 2
		lpos := 0

		if f.Name() == ReferencesFieldName {
			wsep = " "
			lsep = "\r\n "
			lpos = 1
		} else {
			wsep = ", "
			lsep = ",\r\n    "
			lpos = 4
		}

		for i, addr := range f.Addresses {
			a := addr.toString(avoidUtf8)

			if f.Name() == ReferencesFieldName {
				a = "<" + a + ">"
			}

			if first {
				first = false
			} else if (c+len(wsep)+len(a) > 78) ||
				(c+len(wsep)+len(a) == 78 && len(f.Addresses) > i+1) {
				s += lsep
				c = lpos
			} else {
				s += wsep
				c += len(wsep)
			}
			s += a
			c += len(a)
		}
	}

	return s
}

func (f *AddressField) Value() string {
	if len(f.Addresses) == 0 {
		return f.HeaderField.Value()
	}
	// and for message-id, content-id and references:
	v, _ := decode(simplify(f.rfc822(true)), "us-ascii")
	return v
}

func (f *AddressField) Parse(s string) {
	switch f.Name() {
	case SenderFieldName:
		f.parseMailbox(s)
		if !f.Valid() && len(f.Addresses) == 0 {
			// sender is quite often wrong in otherwise perfectly
			// legible messages. so we'll nix out the error. Header
			// will probably remove the field completely, since an
			// empty Sender field isn't sensible.
			f.Error = nil
		}
	case ReturnPathFieldName:
		f.parseMailbox(s)
		if !f.Valid() || len(f.Addresses) != 1 ||
			(f.Addresses[0].t != BounceAddressType && f.Addresses[0].t != NormalAddressType) {
			// return-path sometimes contains strange addresses when
			// migrating from older stores. if it does, just kill
			// it. this never happens when receiving mail, since we'll
			// make a return-path of our own.
			f.Error = nil
			f.Addresses = nil
		}
	case ResentSenderFieldName:
		f.parseMailbox(s)
	case FromFieldName, ResentFromFieldName:
		f.parseMailboxList(s)
	case ToFieldName, CcFieldName, BccFieldName, ReplyToFieldName,
		ResentToFieldName, ResentCcFieldName, ResentBccFieldName:
		f.parseAddressList(s)
		if f.Name() == CcFieldName && !f.Valid() && len(f.Addresses) <= 1 {
			// /bin/mail tempts people to type escape, ctrl-d or
			// similar into the cc field, so we try to recover from
			// that.
			i := 0
			for i < len(s) && s[i] >= ' ' && s[i] != 127 {
				i++
			}
			if i < len(s) {
				f.Error = nil
				f.Addresses = nil
			}
		}
		if !f.Valid() && len(simplify(s)) == 1 {
			f.Error = nil
			f.Addresses = nil
		}
		if f.Valid() && strings.Contains(s, "<>") {
			// some spammers attempt to send 'To: asdfsaf <>'.
			bounces := 0
			otherProblems := 0
			for _, a := range f.Addresses {
				if a.t == BounceAddressType {
					bounces++
				} else if a.err != nil {
					otherProblems++
				}
			}
			if bounces > 0 && otherProblems == 0 {
				// there's one or more <>, but nothing else bad.
				clean := make([]Address, 0, len(f.Addresses)-bounces)
				for _, a := range f.Addresses {
					if a.t != BounceAddressType {
						clean = append(clean, a)
					}
				}
				f.Addresses = clean
			}
			if !f.Valid() && len(f.Addresses) == 0 && !strings.Contains(s, "@") {
				// some spammers send total garbage. we can't detect all
				// instances of garbage, but if it doesn't contain even
				// one "@" and also not even one parsable address, surely
				// it's garbage.
				f.Error = nil
			}
			if !f.Valid() && len(f.Addresses) <= 1 &&
				(strings.HasPrefix(s, "@") || strings.Contains(s, "<@")) {
				f.Addresses = nil
				f.Error = nil
			}
		}
	case ContentIdFieldName:
		f.parseContentId(s)
	case MessageIdFieldName, ResentMessageIdFieldName:
		f.parseMessageId(s)
	case ReferencesFieldName:
		f.parseReferences(s)
	default:
		// Should not happen.
	}

	if f.Name() != ReturnPathFieldName {
		f.outlawBounce()
	}
}

// Parses the RFC 2822 address-list production from \a s and records the first
// problem found.
func (f *AddressField) parseAddressList(s string) {
	ap := NewAddressParser(s)
	f.Error = ap.firstError
	f.Addresses = ap.addresses
}

// Parses the RFC 2822 mailbox-list production from \a s and records the first
// problem found.
func (f *AddressField) parseMailboxList(s string) {
	f.parseAddressList(s)

	// A mailbox-list is an address-list where groups aren't allowed.
	for _, a := range f.Addresses {
		if !f.Valid() {
			break
		}
		if a.t == EmptyGroupAddressType {
			f.Error = fmt.Errorf("Invalid mailbox: %q", a.toString(false))
		}
	}
}

// Parses the RFC 2822 mailbox production from \a s and records the first
// problem found.
func (f *AddressField) parseMailbox(s string) {
	f.parseAddressList(s)

	// A mailbox in our world is just a mailbox-list with one entry.
	if f.Valid() && len(f.Addresses) > 1 {
		f.Error = errors.New("Only one address is allowed")
	}
}

// Parses the contents of an RFC 2822 references field in \a s. This is
// nominally 1*msg-id, but in practice we need to be a little more flexible.
// Overlooks common problems and records the first serious problems found.
func (f *AddressField) parseReferences(s string) {
	ap := references(s)
	f.Addresses = ap.addresses
	f.Error = ap.firstError
}

// Parses the RFC 2822 msg-id production from \a s and/or records the first
// serious error found.
func (f *AddressField) parseMessageId(s string) {
	ap := references(s)

	if ap.firstError != nil {
		f.Error = ap.firstError
	} else if len(ap.addresses) == 1 {
		f.Addresses = ap.addresses
	} else {
		f.Error = errors.New("Need exactly one")
	}
}

// Like parseMessageId( \a s ), except that it also accepts <blah>.
func (f *AddressField) parseContentId(s string) {
	ap := NewAddressParser(s)
	f.Error = ap.firstError
	if len(ap.addresses) != 1 {
		f.Error = errors.New("Need exactly one")
		return
	}

	switch ap.addresses[0].t {
	case NormalAddressType:
		f.Addresses = ap.addresses
	case BounceAddressType:
		f.Error = errors.New("<> is not legal, it has to be <some@thing>")
	case EmptyGroupAddressType:
		f.Error = errors.New("Error parsing Content-Id")
	case LocalAddressType:
		f.Addresses = ap.addresses
	case InvalidAddressType:
		f.Error = errors.New("Error parsing Content-Id")
	}
}

// Checks whether '<>' is present in this address field, and records an error
// if it is. '<>' is legal in Return-Path, but as of April 2005, not in any
// other field.
func (f *AddressField) outlawBounce() {
	for _, a := range f.Addresses {
		if a.t == BounceAddressType {
			f.Error = errors.New("No-bounce address not allowed in this field")
		}
	}
}

// This static function parses the references field \a r. This is in
// AddressParser because References and Message-ID both use the address
// productions in RFC 822/1034.
//
// This function does it best to skip ahead to the next message-id if there is
// a syntax error in one. It silently ignores the errors. This is because it's
// so common to have a bad message-id in the references field of an otherwise
// impeccable message.
func references(r string) AddressParser {
	ap := NewAddressParser("")
	ap.s = r
	i := len(r) - 1
	i = ap.comment(i)
	for i > 0 {
		l := i
		ok := true
		dom := ""
		lp := ""
		if r[i] != '>' {
			ok = false
		} else {
			i--
			dom, i = ap.domain(i)
			if i >= 0 && r[i] == '@' {
				i--
			} else {
				ok = false
			}
			lp, i = ap.localpart(i)
			if i >= 0 && r[i] == '<' {
				i--
			} else {
				ok = false
			}
			i = ap.comment(i)
			if i >= 0 && ap.s[i] == ',' {
				i--
				i = ap.comment(i)
			}
		}
		if ok && dom != "" && lp != "" {
			ap.add("", lp, dom)
		} else {
			i = l
			i--
			for i >= 0 && r[i] != ' ' {
				i--
			}
			i = ap.comment(i)
		}
	}
	ap.firstError = nil
	return ap
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
