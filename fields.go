package mail

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/paulrosania/go-charset/charset"
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

var fieldNames = []string{
	FromFieldName,
	ResentFromFieldName,
	SenderFieldName,
	ResentSenderFieldName,
	ReturnPathFieldName,
	ReplyToFieldName,
	ToFieldName,
	CcFieldName,
	BccFieldName,
	ResentToFieldName,
	ResentCcFieldName,
	ResentBccFieldName,
	MessageIdFieldName,
	ResentMessageIdFieldName,
	InReplyToFieldName,
	ReferencesFieldName,
	DateFieldName,
	OrigDateFieldName,
	ResentDateFieldName,
	SubjectFieldName,
	CommentsFieldName,
	KeywordsFieldName,
	ContentTypeFieldName,
	ContentTransferEncodingFieldName,
	ContentDispositionFieldName,
	ContentDescriptionFieldName,
	ContentIdFieldName,
	MimeVersionFieldName,
	ReceivedFieldName,
	ContentLanguageFieldName,
	ContentLocationFieldName,
	ContentMd5FieldName,
	ListIdFieldName,
	ContentBaseFieldName,
	ErrorsToFieldName,
}

var isKnownField map[string]bool

func init() {
	isKnownField = make(map[string]bool)
	for _, n := range fieldNames {
		isKnownField[n] = true
	}
}

type Field interface {
	Name() string
	Value() string
	Error() error

	Parse(value string)
	Valid() bool
	SetUnparsedValue(value string)

	rfc822(avoidUtf8 bool) string
}

type Fields []Field

func (fs *Fields) RemoveAt(i int) {
	*fs = append((*fs)[:i], (*fs)[i+1:]...)
}

func (fs *Fields) Remove(r Field) {
	for i, f := range *fs {
		if f == r {
			fs.RemoveAt(i)
		}
	}
}

type HeaderField struct {
	name, value   string
	UnparsedValue string
	err           error
}

func (f *HeaderField) Name() string {
	return f.name
}

func (f *HeaderField) Value() string {
	return f.value
}

func (f *HeaderField) Error() error {
	return f.err
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
		f.err = errors.New("Error parsing text")
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
		f.err = fmt.Errorf("Junk at position %d: %s", e, s[e:])
	} else if err != nil {
		f.err = err
	}
}

// Tries to parses any (otherwise uncovered and presumably unstructured) field
// in \a s, and records an error if it contains NULs or 8-bit characters.
func (f *HeaderField) parseOther(s string) {
	v, err := decode(s, "us-ascii")
	if err != nil {
		f.err = err
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
		f.err = errors.New("URL has no scheme")
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
	f.err = err
}

// Returns true if this header field is valid (or unparsed, as is the case for
// all unknown fields), and false if an error was detected during parsing.
func (f *HeaderField) Valid() bool {
	return f.err == nil
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
			f.err = nil
		}
	case ReturnPathFieldName:
		f.parseMailbox(s)
		if !f.Valid() || len(f.Addresses) != 1 ||
			(f.Addresses[0].t != BounceAddressType && f.Addresses[0].t != NormalAddressType) {
			// return-path sometimes contains strange addresses when
			// migrating from older stores. if it does, just kill
			// it. this never happens when receiving mail, since we'll
			// make a return-path of our own.
			f.err = nil
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
				f.err = nil
				f.Addresses = nil
			}
		}
		if !f.Valid() && len(simplify(s)) == 1 {
			f.err = nil
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
				f.err = nil
			}
			if !f.Valid() && len(f.Addresses) <= 1 &&
				(strings.HasPrefix(s, "@") || strings.Contains(s, "<@")) {
				f.Addresses = nil
				f.err = nil
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
	f.err = ap.firstError
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
			f.err = fmt.Errorf("Invalid mailbox: %q", a.toString(false))
		}
	}
}

// Parses the RFC 2822 mailbox production from \a s and records the first
// problem found.
func (f *AddressField) parseMailbox(s string) {
	f.parseAddressList(s)

	// A mailbox in our world is just a mailbox-list with one entry.
	if f.Valid() && len(f.Addresses) > 1 {
		f.err = errors.New("Only one address is allowed")
	}
}

// Parses the contents of an RFC 2822 references field in \a s. This is
// nominally 1*msg-id, but in practice we need to be a little more flexible.
// Overlooks common problems and records the first serious problems found.
func (f *AddressField) parseReferences(s string) {
	ap := references(s)
	f.Addresses = ap.addresses
	f.err = ap.firstError
}

// Parses the RFC 2822 msg-id production from \a s and/or records the first
// serious error found.
func (f *AddressField) parseMessageId(s string) {
	ap := references(s)

	if ap.firstError != nil {
		f.err = ap.firstError
	} else if len(ap.addresses) == 1 {
		f.Addresses = ap.addresses
	} else {
		f.err = errors.New("Need exactly one")
	}
}

// Like parseMessageId( \a s ), except that it also accepts <blah>.
func (f *AddressField) parseContentId(s string) {
	ap := NewAddressParser(s)
	f.err = ap.firstError
	if len(ap.addresses) != 1 {
		f.err = errors.New("Need exactly one")
		return
	}

	switch ap.addresses[0].t {
	case NormalAddressType:
		f.Addresses = ap.addresses
	case BounceAddressType:
		f.err = errors.New("<> is not legal, it has to be <some@thing>")
	case EmptyGroupAddressType:
		f.err = errors.New("Error parsing Content-Id")
	case LocalAddressType:
		f.Addresses = ap.addresses
	case InvalidAddressType:
		f.err = errors.New("Error parsing Content-Id")
	}
}

// Checks whether '<>' is present in this address field, and records an error
// if it is. '<>' is legal in Return-Path, but as of April 2005, not in any
// other field.
func (f *AddressField) outlawBounce() {
	for _, a := range f.Addresses {
		if a.t == BounceAddressType {
			f.err = errors.New("No-bounce address not allowed in this field")
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

// Layouts suitable for passing to time.Parse.
// These are tried in order.
var dateLayouts []string

func init() {
	// Generate layouts based on RFC 5322, section 3.3.

	dows := [...]string{"", "Mon, "}   // day-of-week
	days := [...]string{"2", "02"}     // day = 1*2DIGIT
	years := [...]string{"2006", "06"} // year = 4*DIGIT / 2*DIGIT
	seconds := [...]string{":05", ""}  // second
	// "-0700 (MST)" is not in RFC 5322, but is common.
	zones := [...]string{"-0700", "MST", "-0700 (MST)"} // zone = (("+" / "-") 4DIGIT) / "GMT" / ...

	for _, dow := range dows {
		for _, day := range days {
			for _, year := range years {
				for _, second := range seconds {
					for _, zone := range zones {
						s := dow + day + " Jan " + year + " 15:04" + second + " " + zone
						dateLayouts = append(dateLayouts, s)
					}
				}
			}
		}
	}
}

// TODO: Evaluate aox implementation, might be more lenient
func (f *DateField) Parse(s string) {
	for _, layout := range dateLayouts {
		t, err := time.Parse(layout, s)
		if err == nil {
			f.value = t.Format("Mon, 02 Jan 2006 15:04:05 -0700")
			return
		}
	}
	f.err = errors.New("mail: header could not be parsed")
}

type MimeParameter struct {
	Name, Value string
	Parts       map[int]string
}

func NewMimeParameter(name, value string) MimeParameter {
	return MimeParameter{
		Name:  name,
		Value: value,
		Parts: make(map[int]string),
	}
}

type MimeField struct {
	HeaderField
	baseValue  string
	Parameters []MimeParameter
}

// Returns the value of the parameter named \a n (ignoring the case of the
// name). If there is no such parameter, this function returns an empty string.
func (f *MimeField) parameter(n string) string {
	s := strings.ToLower(n)
	for _, p := range f.Parameters {
		if p.Name == s {
			return p.Value
		}
	}
	return ""
}

// Adds a parameter named \a n with value \a v, replacing any previous setting.
func (f *MimeField) addParameter(n, v string) {
	s := strings.ToLower(n)
	found := false
	for i := 0; i < len(f.Parameters); i++ {
		if f.Parameters[i].Name == s {
			f.Parameters[i].Value = v
			found = true
		}
	}
	if !found {
		p := MimeParameter{Name: n, Value: v}
		f.Parameters = append(f.Parameters, p)
	}
}

// Parses \a p, which is expected to refer to a string whose next characters
// form the RFC 2045 production '*(";"parameter)'.
func (f *MimeField) parseParameters(p *Parser) {
	done := false
	first := true
	for f.Valid() && !done {
		done = true
		i := p.Pos()
		for p.NextChar() == ';' ||
			p.NextChar() == ' ' || p.NextChar() == '\t' ||
			p.NextChar() == '\r' || p.NextChar() == '\n' ||
			p.NextChar() == '"' {
			p.Step(1)
		}
		if i < p.Pos() {
			done = false
		}
		if first {
			done = false
		}
		if p.AtEnd() {
			done = true
		}
		first = false
		if !done {
			n := strings.ToLower(p.MimeToken())
			p.Comment()
			havePart := false
			partNumber := 0

			if n == "" {
				return
			}

			if strings.Contains(n, "*") {
				star := strings.Index(n, "*")
				var err error
				partNumber, err = strconv.Atoi(n[star+1:])
				if err == nil {
					havePart = true
					n = n[:star]
				}
			}
			if f.Name() == ContentTypeFieldName && p.AtEnd() && charset.Info(n) != nil {
				// sometimes we see just iso-8859-1 instead of charset=iso-8859-1.
				exists := false
				for _, param := range f.Parameters {
					if param.Name == "charset" {
						exists = true
						break
					}
				}
				if !exists {
					param := NewMimeParameter("charset", n)
					f.Parameters = append(f.Parameters, param)
					return
				}
			}
			if p.NextChar() == ':' && isKnownField[n] {
				// some spammers send e.g. 'c-t: stuff subject:
				// stuff'.  we ignore the second field entirely. who
				// cares about spammers.
				n = ""
				p.Step(len(p.str))
			} else if p.NextChar() != '=' {
				return
			}

			p.Step(1)
			p.Whitespace()
			v := ""
			if p.NextChar() == '"' {
				v = p.MimeValue()
			} else {
				start := p.Pos()
				v = p.MimeValue()
				ok := true
				for ok && !p.AtEnd() &&
					p.NextChar() != ';' &&
					p.NextChar() != '"' {
					if p.DotAtom() == "" && p.MimeValue() == "" {
						ok = false
					}
				}
				if ok {
					v = p.str[start:p.Pos()]
				}
			}
			p.Comment()

			if n != "" {
				i := 0
				for i < len(f.Parameters) {
					if f.Parameters[i].Name == n {
						break
					}
					i++
				}
				if i >= len(f.Parameters) {
					param := NewMimeParameter(n, "")
					f.Parameters = append(f.Parameters, param)
				}
				if havePart {
					f.Parameters[i].Parts[partNumber] = v
				} else {
					f.Parameters[i].Value = v
				}
			}
		}
	}

	for _, p := range f.Parameters {
		if p.Value == "" && p.Parts[0] != "" { // TODO: should probably test presence rather than emptiness
			// I get to be naughty too sometimes
			n := 0
			v, ok := p.Parts[n]
			for ok {
				p.Value += v
				n++
				v = p.Parts[n]
			}
		}
	}
}

// This reimplementation of rfc822() never generates UTF-8 at the moment.
// Merely a SMoP, but I haven't the guts to do it at the moment.
func (f *MimeField) rfc822(avoidUtf8 bool) string {
	s := f.baseValue
	lineLength := len(f.Name()) + 2 + len(s)

	words := []string{}
	for _, p := range f.Parameters {
		s := p.Value
		if !isBoring(s, MIMEBoring) {
			s = quote(s, '"', '\'')
		}
		words = append(words, p.Name+"="+s)
	}

	for len(words) > 0 {
		i := 0
		for i < len(words) && lineLength+2+len(words[i]) > 78 {
			i++
		}
		if i < len(words) {
			s += "; "
			lineLength += 2
		} else {
			i = 0
			s += ";\r\n "
			lineLength = 1
		}
		s += words[i] // FIXME: need more elaboration for 2231
		lineLength += len(words[i])
		words = append(words[:i], words[i+1:]...)
	}

	return s
}

// Like HeaderField::value(), returns the contents of this MIME field in a
// representation suitable for storage.
func (f *MimeField) Value() string {
	return f.rfc822(false)
	// the best that can be said about this is that it corresponds to
	// HeaderField::assemble.
}

type ContentType struct {
	MimeField
	Type, Subtype string
}

func NewContentType() *ContentType {
	hf := HeaderField{name: ContentTypeFieldName}
	mf := MimeField{HeaderField: hf}
	return &ContentType{MimeField: mf}
}

func (f *ContentType) Parse(s string) {
	p := NewParser(s)
	p.Whitespace()
	for p.Present(":") {
		p.Whitespace()
	}

	mustGuess := false

	if p.AtEnd() {
		f.Type = "text"
		f.Subtype = "plain"
	} else {
		x := p.mark()
		if p.NextChar() == '/' {
			mustGuess = true
		} else {
			f.Type = strings.ToLower(p.MimeToken())
		}
		if p.AtEnd() {
			if s == "text" {
				f.Type = "text" // elm? mailtool? someone does this, anyway.
				f.Subtype = "plain"
				// the remainder is from RFC 1049
			} else if s == "postscript" {
				f.Type = "application"
				f.Subtype = "postscript"
			} else if s == "sgml" {
				f.Type = "text"
				f.Subtype = "sgml"
			} else if s == "tex" {
				f.Type = "application"
				f.Subtype = "x-tex"
			} else if s == "troff" {
				f.Type = "application"
				f.Subtype = "x-troff"
			} else if s == "dvi" {
				f.Type = "application"
				f.Subtype = "x-dvi"
			} else if strings.HasPrefix(s, "x-") {
				f.Type = "application"
				f.Subtype = "x-rfc1049-" + s
			} else {
				// scribe and undefined types
				f.err = fmt.Errorf("Invalid Content-Type: %q", s)
			}
		} else {
			if p.NextChar() == '/' {
				p.Step(1)
				if !p.AtEnd() || p.NextChar() != ';' {
					f.Subtype = strings.ToLower(p.MimeToken())
				}
				if f.Subtype == "" {
					mustGuess = true
				}
			} else if p.NextChar() == '=' {
				// oh no. someone skipped the content-type and
				// supplied only some parameters. we'll assume it's
				// text/plain and parse the parameters.
				f.Type = "text"
				f.Subtype = "plain"
				p.restore(x)
				mustGuess = true
			} else {
				f.addParameter("original-type", f.Type+"/"+f.Subtype)
				f.Type = "application"
				f.Subtype = "octet-stream"
				mustGuess = true
			}
			f.parseParameters(p)
		}
	}

	if mustGuess {
		fn := f.parameter("name")
		if fn == "" {
			fn = f.parameter("filename")
		}
		for strings.HasSuffix(fn, ".") {
			fn = fn[:len(fn)-1]
		}
		fn = strings.ToLower(fn)
		if strings.HasSuffix(fn, "jpg") || strings.HasSuffix(fn, "jpeg") {
			f.Type = "image"
			f.Subtype = "jpeg"
		} else if strings.HasSuffix(fn, "htm") || strings.HasSuffix(fn, "html") {
			f.Type = "text"
			f.Subtype = "html"
		} else if fn == "" && f.Subtype == "" && f.Type == "text" {
			f.Subtype = "plain"
		} else if f.Type == "text" {
			f.addParameter("original-type", f.Type+"/"+f.Subtype)
			f.Subtype = "plain"
		} else {
			f.addParameter("original-type", f.Type+"/"+f.Subtype)
			f.Type = "application"
			f.Subtype = "octet-stream"
		}
	}

	if f.Type == "" || f.Subtype == "" {
		f.err = fmt.Errorf("Both type and subtype must be nonempty: %q" + s)
	}

	if f.Valid() && f.Type == "multipart" && f.Subtype == "appledouble" &&
		f.parameter("boundary") == "" {
		// some people send appledouble without the header. what can
		// we do? let's just call it application/octet-stream. whoever
		// wants to decode can try, or reply.
		f.Type = "application"
		f.Subtype = "octet-stream"
	}

	if f.Valid() && !p.AtEnd() &&
		f.Type == "multipart" && f.parameter("boundary") == "" &&
		containsWord(strings.ToLower(s), "boundary") {
		csp := NewParser(s[strings.Index(strings.ToLower(s), "boundary"):])
		csp.require("boundary")
		csp.Whitespace()
		if csp.Present("=") {
			csp.Whitespace()
		}
		m := csp.mark()
		b := csp.String()
		if b == "" || csp.err != nil {
			csp.restore(m)
			b = simplify(section(csp.str[csp.Pos():], ";", 1))
			if !isQuoted(b, '"', '\'') {
				b = strings.Replace(b, "\\", "", -1)
			}
			if isQuoted(b, '"', '\'') {
				b = unquote(b, '"', '\'')
			} else if isQuoted(b, '\'', '\'') {
				b = unquote(b, '\'', '\'')
			}
		}
		if b != "" {
			f.addParameter("boundary", b)
		}
	}

	if f.Valid() && f.Type == "multipart" && f.parameter("boundary") == "" {
		f.err = errors.New("Multipart entities must have a boundary parameter.")
	}
	f.baseValue = f.Type + "/" + f.Subtype
}

type ContentTransferEncoding struct {
	MimeField
	Encoding EncodingType
}

func NewContentTransferEncoding() *ContentTransferEncoding {
	hf := HeaderField{name: ContentTransferEncodingFieldName}
	mf := MimeField{HeaderField: hf}
	return &ContentTransferEncoding{MimeField: mf}
}

func (f *ContentTransferEncoding) Parse(s string) {
	p := NewParser(s)

	t := strings.ToLower(p.MimeValue())
	p.Comment()
	// FIXME: shouldn't we do p.end() here and record parse errors?

	if t == "7bit" || t == "8bit" || t == "8bits" || t == "binary" || t == "unknown" {
		f.Encoding = BinaryEncoding
		f.baseValue = "7bit"
	} else if t == "quoted-printable" {
		f.Encoding = QPEncoding
		f.baseValue = "quoted-printable"
	} else if t == "base64" {
		f.Encoding = Base64Encoding
		f.baseValue = "base64"
	} else if t == "x-uuencode" || t == "uuencode" {
		f.Encoding = UuencodeEncoding
		f.baseValue = "x-uuencode"
	} else if strings.Contains(t, "bit") && t[0] >= '0' && t[0] <= '9' {
		f.Encoding = BinaryEncoding
		f.baseValue = "7bit"
	} else {
		f.err = fmt.Errorf("Invalid c-t-e value: %q", t)
	}
}

type ContentDisposition struct {
	MimeField
	Disposition string
}

func NewContentDisposition() *ContentDisposition {
	hf := HeaderField{name: ContentDispositionFieldName}
	mf := MimeField{HeaderField: hf}
	return &ContentDisposition{MimeField: mf}
}

func (f *ContentDisposition) Parse(s string) {
	p := NewParser(s)

	m := p.mark()
	t := strings.ToLower(p.MimeToken())
	p.Whitespace()
	if p.NextChar() == '=' && t != "inline" && t != "attachment" {
		p.restore(m) // handle c-d: filename=foo
	}

	if t == "" {
		f.err = errors.New("Invalid disposition")
		return
	}
	f.parseParameters(p)

	// We are required to treat unknown types as "attachment". If they
	// are syntactically invalid, we replace them with "attachment". (RFC 2183)
	if t != "inline" && t != "attachment" {
		f.Disposition = "attachment"
	} else {
		f.Disposition = t
	}
	f.baseValue = f.Disposition
}

type ContentLanguage struct {
	MimeField
	Languages []string
}

func NewContentLanguage() *ContentLanguage {
	hf := HeaderField{name: ContentLanguageFieldName}
	mf := MimeField{HeaderField: hf}
	return &ContentLanguage{MimeField: mf}
}

func (f *ContentLanguage) Parse(s string) {
	p := NewParser(s)
	for {
		p.Comment()
		t := p.MimeToken()
		if t != "" {
			f.Languages = append(f.Languages, t)
		}
		p.Comment()
		if !p.Present(",") {
			break
		}
	}

	if !p.AtEnd() || len(f.Languages) == 0 {
		f.err = fmt.Errorf("Unparseable value: %q", s)
	}

	f.baseValue = strings.Join(f.Languages, ", ")
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

// Returns the RFC 2822 representation of this header field, with its contents
// properly folded and, if necessary, RFC 2047 encoded. This is a string we can
// hand out to clients.
//
// If \a avoidUtf8 is true, rfc822() avoids UTF-8 in the result, even at the
// cost of losing information.
func (f *HeaderField) rfc822(avoidUtf8 bool) string {
	if f.Name() == SubjectFieldName ||
		f.Name() == CommentsFieldName ||
		f.Name() == ContentDescriptionFieldName {
		if avoidUtf8 {
			return wrap(encodeText(f.value), 78, "", " ", false)
		} else {
			return wrap(f.value, 78, "", " ", false)
		}
	}

	// We assume that, for most fields, we can use the database
	// representation in an RFC 822 message.
	return f.value
}
