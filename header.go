package mail

import (
	"bytes"
	"fmt"
	"math"
	"strings"
	"time"

	"encoding/json"
)

type headerMode int

const (
	RFC5322Header headerMode = iota
	MIMEHeader
)

type defaultContentType int

const (
	TextPlainContentType defaultContentType = iota // default
	MessageRFC822ContentType
)

type Header struct {
	Fields []Field

	defaultType defaultContentType

	mode headerMode

	numBytes int

	err      error
	verified bool
}

func (h *Header) MarshalJSON() ([]byte, error) {
	hs := make([]map[string]interface{}, 0, 8)
	for _, f := range h.Fields {
		h := make(map[string]interface{})
		h["name"] = f.Name()
		h["value"] = f.Value()
		hs = append(hs, h)
	}
	return json.Marshal(hs)
}

func (h *Header) UnmarshalJSON(data []byte) error {
	hs := make([]map[string]interface{}, 0, 8)
	err := json.Unmarshal(data, &hs)
	if err != nil {
		return err
	}

	for _, f := range hs {
		// TODO: Should we handle failed type assertions differently?
		name, ok := f["name"].(string)
		value, ok := f["value"].(string)
		if ok {
			h.Add(name, value)
		}
	}

	return nil
}

func ReadHeader(rfc5322 string, m headerMode) (h *Header, err error) {
	h = &Header{mode: m}
	done := false

	i := 0
	end := len(rfc5322)

	for !done {
		if i >= end {
			done = true
		}

		// Skip past UTF8 byte order mark (BOM) if present
		if i+2 < end && rfc5322[i] == 0xEF && rfc5322[i+1] == 0xBB && rfc5322[i+2] == 0xBF {
			i += 3
		}

		j := i
		for j < end && rfc5322[j] >= 33 && rfc5322[j] <= 127 && rfc5322[j] != ':' {
			j++
		}

		if j == i+4 && m == RFC5322Header && strings.ToLower(rfc5322[i:j+1]) == "from " {
			for i < end && rfc5322[i] != '\r' && rfc5322[i] != '\n' {
				i++
			}
			for rfc5322[i] == '\r' {
				i++
			}
			if rfc5322[i] == '\n' {
				i++
			}
		} else if j > i && rfc5322[j] == ':' {
			name := rfc5322[i:j]
			i = j
			i++
			for rfc5322[i] == ' ' || rfc5322[i] == '\t' {
				i++
			}
			j = i

			// Find the end of the value, including multiline values
			// NOTE: Deviates from https://github.com/aox/aox/blob/master/message/message.cpp#L224
			for j < end && (rfc5322[j] != '\n' || (j+1 < end && (rfc5322[j+1] == ' ' || rfc5322[j+1] == '\t'))) {
				j++
			}
			if j > 0 && rfc5322[j-1] == '\r' {
				j--
			}
			value := rfc5322[i:j]
			//233-237
			if simplify(value) != "" || strings.HasPrefix(strings.ToLower(name), "x-") {
				h.Add(name, value)
			}
			i = j
			if i+1 < end && rfc5322[i] == '\r' && rfc5322[i+1] == '\n' {
				i++
			}
			i++
		} else {
			done = true
		}
	}

	// PR: chomped second newline at header end
	if i+1 < len(rfc5322) && rfc5322[i] == '\r' && rfc5322[i+1] == '\n' {
		i += 2
	} else if i < len(rfc5322) && rfc5322[i] == '\n' {
		i++
	}

	h.numBytes = i

	return h, nil
}

// Returns true if this Header fills all the conditions laid out in RFC 2821
// for validity, and false if not.
func (h *Header) Valid() bool {
	h.verify()
	return h.err == nil
}

// Add adds the key, value pair to the header. It appends to any existing
// values associated with the key.
func (h *Header) Add(key, value string) {
	h.addField(NewHeaderField(key, value))
}

func (h *Header) addField(f Field) {
	if f.Name() == ToFieldName || f.Name() == CcFieldName ||
		f.Name() == BccFieldName || f.Name() == ReplyToFieldName ||
		f.Name() == FromFieldName {
		first := h.addressField(f.Name(), 0)
		next := f.(*AddressField)
		if first != nil {
			for _, a := range next.Addresses {
				first.Addresses = append(first.Addresses, a)
			}
			return
		}
	}
	// TODO: aox implementation allows insertion at specified position
	h.Fields = append(h.Fields, f)
	h.verified = false
}

func (h *Header) RemoveAt(i int) {
	h.Fields = append(h.Fields[:i], h.Fields[i+1:]...)
}

func (h *Header) Remove(r Field) {
	for i, f := range h.Fields {
		if f == r {
			h.RemoveAt(i)
		}
	}
}

func (h *Header) RemoveAllNamed(name string) {
	i := 0
	name = strings.ToLower(name)
	for i < len(h.Fields) {
		if strings.ToLower(h.Fields[i].Name()) == name {
			h.RemoveAt(i)
		} else {
			i++
		}
	}
}

// Get gets the first value associated with the given key. If there are no
// values associated with the key, Get returns "".
func (h *Header) Get(key string) string {
	f := h.field(key, 0)
	if f == nil {
		return ""
	} else {
		return f.Value()
	}
}

func (h *Header) field(fn string, n int) Field {
	for _, field := range h.Fields {
		if field.Name() == fn {
			if n > 0 {
				n--
			} else {
				return field
			}
		}
	}

	return nil
}

// Returns a pointer to the address field of type \a t at index \a n in this
// header, or a null pointer if no such field exists.
func (h *Header) addressField(fn string, n int) *AddressField {
	switch fn {
	case FromFieldName, ResentFromFieldName, SenderFieldName, ResentSenderFieldName,
		ReturnPathFieldName, ReplyToFieldName, ToFieldName, CcFieldName, BccFieldName,
		ResentToFieldName, ResentCcFieldName, ResentBccFieldName, MessageIDFieldName,
		ContentIDFieldName, ResentMessageIDFieldName, ReferencesFieldName:
		f, _ := h.field(fn, n).(*AddressField)
		return f
	}
	return nil
}

// Returns the header's data \a t, which is the normal date by default, but can
// also be orig-date or resent-date. If there is no such field or \a t is
// meaningless, date() returns a null pointer.
func (h *Header) Date() *time.Time {
	hf := h.field(DateFieldName, 0).(*DateField)
	if hf == nil {
		return nil
	}
	return hf.Date
}

// Returns the value of the first Subject header field. If there is no such
// field, returns the empty string.
func (h *Header) Subject() string {
	f := h.field(SubjectFieldName, 0)
	if f == nil {
		return ""
	}

	return f.Value()
}

// Returns a pointer to the addresses in the \a t header field, which must be
// an address field such as From or Bcc. If not, or if the field is empty,
// addresses() returns a null pointer.
func (h *Header) Addresses(fn string) []Address {
	af := h.addressField(fn, 0)
	if af == nil {
		return nil
	}
	return af.Addresses
}

// Returns a pointer to the Content-Type header field, or a null pointer if
// there isn't one.
func (h *Header) ContentType() *ContentType {
	f := h.field(ContentTypeFieldName, 0)
	if f == nil {
		return nil
	}

	return f.(*ContentType)
}

// Returns a pointer to the Content-Transfer-Encoding header field, or a null
// pointer if there isn't one.
func (h *Header) ContentTransferEncoding() *ContentTransferEncoding {
	f := h.field(ContentTransferEncodingFieldName, 0)
	if f == nil {
		return nil
	}

	return f.(*ContentTransferEncoding)
}

// Returns a pointer to the Content-Disposition header field, or a null pointer
// if there isn't one.
func (h *Header) ContentDisposition() *ContentDisposition {
	f := h.field(ContentDispositionFieldName, 0)
	if f == nil {
		return nil
	}

	return f.(*ContentDisposition)
}

// Returns the value of the Content-Description field, or an empty string if
// there isn't one. RFC 2047 encoding is not considered - should it be?
func (h *Header) ContentDescription() string {
	f := h.field(ContentDescriptionFieldName, 0)
	if f == nil {
		return ""
	}
	return simplify(f.rfc822(false))
}

// Returns the value of the Content-Location field, or an empty string if there
// isn't one. The URI is not validated in any way.
func (h *Header) ContentLocation() string {
	f := h.field(ContentLocationFieldName, 0)
	if f == nil {
		return ""
	}
	return f.rfc822(false)
}

// Returns a pointer to the Content-Language header field, or a null pointer if
// there isn't one.
func (h *Header) ContentLanguage() *ContentLanguage {
	f := h.field(ContentLanguageFieldName, 0)
	if f == nil {
		return nil
	}

	return f.(*ContentLanguage)
}

// Returns the value of the Message-ID field, or an empty string if there isn't one
// or if there are multiple (which is illegal).
func (h *Header) MessageID() string {
	ids := h.Addresses(MessageIDFieldName)
	if len(ids) != 1 {
		return ""
	}

	id := ids[0]
	return fmt.Sprintf("<%s@%s>", id.Localpart, id.Domain)
}

func (h *Header) ToMap() map[string][]string {
	headers := make(map[string][]string)
	for _, f := range h.Fields {
		headers[f.Name()] = append(headers[f.Name()], f.Value())
	}
	return headers
}

type HeaderFieldCondition struct {
	name     string
	min, max int
	m        headerMode
}

var conditions = []HeaderFieldCondition{
	HeaderFieldCondition{SenderFieldName, 0, 1, RFC5322Header},
	HeaderFieldCondition{ReplyToFieldName, 0, 1, RFC5322Header},
	HeaderFieldCondition{ToFieldName, 0, 1, RFC5322Header},
	HeaderFieldCondition{CcFieldName, 0, 1, RFC5322Header},
	HeaderFieldCondition{BccFieldName, 0, 1, RFC5322Header},
	HeaderFieldCondition{MessageIDFieldName, 0, 1, RFC5322Header},
	HeaderFieldCondition{ReferencesFieldName, 0, 1, RFC5322Header},
	HeaderFieldCondition{SubjectFieldName, 0, 1, RFC5322Header},
	HeaderFieldCondition{FromFieldName, 1, 1, RFC5322Header},
	HeaderFieldCondition{DateFieldName, 1, 1, RFC5322Header},
	HeaderFieldCondition{MIMEVersionFieldName, 0, 1, RFC5322Header},
	HeaderFieldCondition{MIMEVersionFieldName, 0, 1, MIMEHeader},
	HeaderFieldCondition{ContentTypeFieldName, 0, 1, RFC5322Header},
	HeaderFieldCondition{ContentTypeFieldName, 0, 1, MIMEHeader},
	HeaderFieldCondition{ContentTransferEncodingFieldName, 0, 1, RFC5322Header},
	HeaderFieldCondition{ContentTransferEncodingFieldName, 0, 1, MIMEHeader},
	HeaderFieldCondition{ReturnPathFieldName, 0, 1, RFC5322Header},
}

// This private function verifies that the entire header is consistent and
// legal, and that each contained HeaderField is legal.
func (h *Header) verify() {
	if h.verified {
		return
	}
	h.verified = true
	h.err = nil

	for _, f := range h.Fields {
		if !f.Valid() {
			h.err = fmt.Errorf("%s: %s", f.Name(), f.Error())
			return
		}
	}

	occurrences := make(map[string]int)
	for _, f := range h.Fields {
		occurrences[f.Name()]++
	}

	i := 0
	for h.err == nil && i < len(conditions) {
		if conditions[i].m == h.mode &&
			occurrences[conditions[i].name] < conditions[i].min ||
			occurrences[conditions[i].name] > conditions[i].max {
			if conditions[i].max < occurrences[conditions[i].name] {
				h.err = fmt.Errorf("%d %s fields seen. At most %d may be present.",
					occurrences[conditions[i].name], conditions[i].name, conditions[i].max)
			} else {
				h.err = fmt.Errorf("%d %s fields seen. At least %d must be present.",
					occurrences[conditions[i].name], conditions[i].name, conditions[i].min)
			}
		}
		i++
	}

	// strictly speaking, if From contains more than one address,
	// sender should contain one. we don't enforce that, because it
	// causes too much spam to be rejected that would otherwise go
	// through. we'll filter spam with something that's a little less
	// accidental, and which does not clutter up the logs with so many
	// misleading error messages.

	// we graciously ignore all the Resent-This-Or-That restrictions.
}

func sameAddresses(a, b *AddressField) bool {
	if a == nil || b == nil {
		return false
	}

	l := a.Addresses
	m := b.Addresses

	if l == nil || m == nil {
		return false
	}

	if len(l) != len(m) {
		return false
	}

	lmap := make(map[string]bool)
	for _, a := range l {
		n := fmt.Sprintf("%s@%s", a.Localpart, strings.ToTitle(a.Domain))
		lmap[n] = true
	}

	mmap := make(map[string]bool)
	for _, a := range l {
		n := fmt.Sprintf("%s@%s", a.Localpart, strings.ToTitle(a.Domain))
		mmap[n] = true
	}

	for _, a := range l {
		n := fmt.Sprintf("%s@%s", a.Localpart, strings.ToTitle(a.Domain))
		if !mmap[n] {
			return false
		}
	}

	for _, a := range m {
		n := fmt.Sprintf("%s@%s", a.Localpart, strings.ToTitle(a.Domain))
		if !lmap[n] {
			return false
		}
	}

	return true
}

// Removes any redundant header fields from this header, and simplifies the
// value of some.
//
// For example, if 'sender' or 'reply-to' points to the same address as 'from',
// that field can be removed, and if 'from' contains the same address twice,
// one can be removed.
func (h *Header) Simplify() {
	if !h.Valid() {
		return
	}

	for _, fn := range addressFieldNames {
		af := h.addressField(fn, 0)
		if af != nil {
			af.Addresses.Uniquify()
		}
	}

	cde := h.field(ContentDescriptionFieldName, 0)
	if cde != nil && cde.rfc822(false) == "" {
		h.RemoveAllNamed(ContentDescriptionFieldName)
		cde = nil
	}

	cte := h.ContentTransferEncoding()
	if cte != nil && cte.Encoding == BinaryEncoding {
		h.RemoveAllNamed(ContentTransferEncodingFieldName)
	}

	cdi := h.ContentDisposition()
	if cdi != nil {
		ct := h.ContentType()
		if h.mode == RFC5322Header && (ct == nil || ct.Type == "text") &&
			cdi.Disposition == "inline" &&
			len(cdi.Parameters) == 0 {
			h.RemoveAllNamed(ContentDispositionFieldName)
			cdi = nil
		}
	}

	ct := h.ContentType()
	if ct != nil {
		if len(ct.Parameters) == 0 && cte == nil && cdi == nil && cde == nil &&
			h.defaultType == TextPlainContentType &&
			ct.Type == "text" && ct.Subtype == "plain" {
			h.RemoveAllNamed(ContentTypeFieldName)
			ct = nil
		}
	} else if h.defaultType == MessageRFC822ContentType {
		h.Add("Content-Type", "message/rfc822")
		ct = h.ContentType()
	}

	if h.mode == MIMEHeader {
		h.RemoveAllNamed(MIMEVersionFieldName)
	} else if ct == nil && cte == nil && cde == nil && cdi == nil &&
		h.field(ContentLocationFieldName, 0) == nil &&
		h.field(ContentBaseFieldName, 0) == nil {
		h.RemoveAllNamed(MIMEVersionFieldName)
	} else {
		if h.mode == RFC5322Header && h.field(MIMEVersionFieldName, 0) == nil {
			h.Add("MIME-Version", "1.0")
		}
	}
	if ct != nil &&
		(ct.Type == "multipart" || ct.Type == "message" ||
			ct.Type == "image" || ct.Type == "audio" ||
			ct.Type == "video") {
		ct.removeParameter("charset")
	}

	if h.field(ErrorsToFieldName, 0) != nil {
		et := ascii(h.field(ErrorsToFieldName, 0).Value())
		rp := h.Addresses(ReturnPathFieldName)
		if rp != nil && len(rp) == 1 &&
			strings.ToLower(rp[0].lpdomain()) == strings.ToLower(et) {
			h.RemoveAllNamed(ErrorsToFieldName)
		}
	}

	m := h.field(MessageIDFieldName, 0)
	if m != nil && m.rfc822(false) == "" {
		h.RemoveAllNamed(MessageIDFieldName)
	}

	if sameAddresses(h.addressField(FromFieldName, 0), h.addressField(ReplyToFieldName, 0)) {
		h.RemoveAllNamed(ReplyToFieldName)
	}

	if sameAddresses(h.addressField(FromFieldName, 0), h.addressField(SenderFieldName, 0)) {
		h.RemoveAllNamed(SenderFieldName)
	}

	if len(h.Addresses(SenderFieldName)) == 0 {
		h.RemoveAllNamed(SenderFieldName)
	}
	if len(h.Addresses(ReturnPathFieldName)) == 0 {
		h.RemoveAllNamed(ReturnPathFieldName)
	}
	if len(h.Addresses(ToFieldName)) == 0 {
		h.RemoveAllNamed(ToFieldName)
	}
	if len(h.Addresses(CcFieldName)) == 0 {
		h.RemoveAllNamed(CcFieldName)
	}
	if len(h.Addresses(BccFieldName)) == 0 {
		h.RemoveAllNamed(BccFieldName)
	}
	if len(h.Addresses(ReplyToFieldName)) == 0 {
		h.RemoveAllNamed(ReplyToFieldName)
	}
}

// Repairs problems that can be repaired without knowing the associated
// bodypart.
func (h *Header) Repair() {
	if h.Valid() {
		return
	}

	// We remove duplicates of any field that may occur only once.
	// (Duplication has been observed for Date/Subject/M-V/C-T-E/C-T/M-I.)

	occurrences := make(map[string]int)
	for _, f := range h.Fields {
		occurrences[f.Name()]++
	}

	i := 0
	for i < len(conditions) {
		if conditions[i].m == h.mode &&
			occurrences[conditions[i].name] > conditions[i].max {
			n := 0
			j := 0
			hf := h.field(conditions[i].name, 0)
			for j < len(h.Fields) {
				if h.Fields[j].Name() == conditions[i].name {
					n++
					if n > 1 && hf.rfc822(false) == h.Fields[j].rfc822(false) {
						h.RemoveAt(j)
					} else {
						j++
					}
				} else {
					j++
				}
			}
		}
		i++
	}

	// If there are several content-type fields, and they agree except
	// that one has options and the others not, remove the option-less
	// ones.

	if occurrences[ContentTypeFieldName] > 1 {
		ct := h.ContentType()
		other := ct
		var good *ContentType
		n := 0
		bad := false
		for other != nil && !bad {
			if other.Type != ct.Type ||
				other.Subtype != ct.Subtype {
				bad = true
			} else if len(other.Parameters) > 0 {
				if good != nil {
					bad = true
				}
				good = other
			}
			n++
			tmp := h.field(ContentTypeFieldName, n)
			if tmp != nil {
				other = tmp.(*ContentType)
			} else {
				other = nil
			}
		}
		if good != nil && !bad {
			i := 0
			for i < len(h.Fields) {
				if h.Fields[i].Name() == ContentTypeFieldName && h.Fields[i] != good {
					h.RemoveAt(i)
				} else {
					i++
				}
			}
		}
	}

	// We retain only the first valid Date field, Return-Path,
	// Message-Id, References and Content-Type fields. If there is one
	// or more valid such field, we delete all invalid fields,
	// otherwise we leave the fields as they are.

	// For most of these, we also delete subsequent valid fields. For
	// Content-Type we only delete invalid fields, since there isn't
	// any strong reason to believe that the one we would keep enables
	// correct interpretation of the body.

	// Several senders appear to send duplicate dates. qmail is
	// mentioned in the references chains of most examples we have.

	// We don't know who adds duplicate message-id, return-path and
	// content-type fields.

	// The only case we've seen of duplicate references involved
	// Thunderbird 1.5.0.4 and Scalix. Uncertain whose
	// bug. Thunderbird 1.5.0.5 looks correct.

	for _, name := range fieldNames {
		if occurrences[name] > 1 &&
			(name == DateFieldName ||
				name == ReturnPathFieldName ||
				name == MessageIDFieldName ||
				name == ContentTypeFieldName ||
				name == ReferencesFieldName) {
			var firstValid Field
			for _, f := range h.Fields {
				if f.Name() == name && f.Valid() {
					firstValid = f
					break
				}
			}
			if firstValid != nil {
				alsoValid := true
				if name == ContentTypeFieldName {
					alsoValid = false
				}
				i := 0
				for i < len(h.Fields) {
					if h.Fields[i].Name() == name && h.Fields[i] != firstValid &&
						(alsoValid || !h.Fields[i].Valid()) {
						h.RemoveAt(i)
					} else {
						i++
					}
				}
			}
		}
	}

	// MIME-Version is occasionally seen more than once, usually on
	// spam or mainsleaze.
	if h.field(MIMEVersionFieldName, 1) != nil {
		h.Remove(h.field(MIMEVersionFieldName, 1))
		fmv := h.field(MIMEVersionFieldName, 0)
		fmv.Parse(fmt.Sprintf("1.0 (Note: original message contained %d MIME-Version fields)", occurrences[MIMEVersionFieldName]))
	}

	// Content-Transfer-Encoding: should not occur on multiparts, and
	// when it does it usually has a syntax error. We don't care about
	// that error.
	if occurrences[ContentTransferEncodingFieldName] > 0 {
		ct := h.ContentType()
		if ct != nil && ct.Type == "multipart" || ct.Type == "message" {
			h.RemoveAllNamed(ContentTransferEncodingFieldName)
		}
	}

	// Sender sometimes is a straight copy of From, even if From
	// contains more than one address. If it's a copy, or even an
	// illegal subset, we drop it.

	senders := h.Addresses(SenderFieldName)

	if occurrences[SenderFieldName] > 0 && len(senders) != 1 {
		from := make(map[string]bool)
		for _, a := range h.Addresses(FromFieldName) {
			from[strings.ToLower(a.lpdomain())] = true
		}

		sender := []string{}
		for _, a := range h.Addresses(FromFieldName) {
			sender = append(sender, strings.ToLower(a.lpdomain()))
		}

		i := 0
		difference := false
		for i < len(sender) && difference {
			if !from[sender[i]] {
				difference = true
			}
			i++
		}
		if !difference {
			h.RemoveAllNamed(SenderFieldName)
		}
	}
}

// Repairs a few harmless and common problems, such as inserting two Date
// fields with the same value. Assumes that \a p is its companion body (whose
// text is in \a body), and may look at it to decide what/how to repair.
func (h *Header) RepairWithBody(p *Part, body string) {
	if h.Valid() {
		return
	}

	// Duplicated from above.
	occurrences := make(map[string]int)
	for _, f := range h.Fields {
		occurrences[f.Name()]++
	}

	i := 0
	for i < len(conditions) {
		if conditions[i].m == h.mode &&
			occurrences[conditions[i].name] > conditions[i].max {
			n := 0
			j := 0
			hf := h.field(conditions[i].name, 0)
			for j < len(h.Fields) {
				if h.Fields[j].Name() == conditions[i].name {
					n++
					if n > 1 && hf.rfc822(false) == h.Fields[j].rfc822(false) {
						h.RemoveAt(j)
					} else {
						j++
					}
				} else {
					j++
				}
			}
		}
		i++
	}

	// If there is no valid Date field and this is an RFC822 header,
	// we look for a sensible date.

	if h.mode == RFC5322Header &&
		(occurrences[DateFieldName] == 0 ||
			!h.field(DateFieldName, 0).Valid() ||
			h.Date() != nil) {
		var date *time.Time
		for _, f := range h.Fields {
			// First, we take the date from the oldest plausible
			// Received field.
			if f.Name() == ReceivedFieldName {
				v := f.rfc822(false)
				i := 0
				for strings.Index(v[i+1:], ";") > 0 {
					i = i + 1 + strings.Index(v[i+1:], ";")
				}
				if i >= 0 {
					tmp := parseDate(v[i+1:])
					if tmp != nil {
						if date == nil {
							// first plausible we've seen
							date = tmp
						} else {
							// if it took more than an hour to
							// deliver, or less than no time, we don't
							// trust this received field at all.
							// FIXME: aox has a buggy extra comparison here, do we need it?
							if tmp.Before(*date) {
								date = tmp
							}
						}
					}
				}
			}
		}

		if date == nil && p != nil {
			parent := p.parent
			for parent != nil && parent.Header != nil && parent.Header.Date() == nil {
				parent = parent.parent
			}
			if parent != nil {
				date = parent.Header.Date()
			}
		}

		if date == nil && occurrences[DateFieldName] == 0 {
			// Try to see if the top-level message has an internaldate,
			// just in case it might be valid.
			parent := p
			for parent != nil && parent.parent != nil {
				parent = parent.parent
			}
			// FIXME: reference message internalDate or remove this clause
		}

		if date == nil && occurrences[DateFieldName] == 0 {
			// As last resort, use the current date, time and
			// timezone.  Only do this if there isn't a date field. If
			// there is one, we'll reject the message (at least for
			// now) since this happens only for submission in
			// practice.
			tmp := time.Now()
			date = &tmp
		}

		if date != nil {
			// FIXME: aox inserts at position of existing field, or at end
			h.Add(DateFieldName, date.Format(time.RFC822Z))
		}
	}

	// If there is no From field, try to use either Return-Path or
	// Sender from this Header, or From, Return-Path or Sender from
	// the Header of the closest encompassing Multipart that has such
	// a field.

	if occurrences[FromFieldName] == 0 && h.mode == RFC5322Header {
		parent := p
		head := h
		a := []Address{}
		for (head != nil || parent != nil) && len(a) == 0 {
			if head != nil {
				a = h.Addresses(FromFieldName)
			}
			if head != nil && (len(a) == 0 || a[0].t != NormalAddressType) {
				a = h.Addresses(ReturnPathFieldName)
			}
			if head != nil && (len(a) == 0 || a[0].t != NormalAddressType) {
				a = h.Addresses(SenderFieldName)
			}
			if head != nil && (len(a) == 0 || a[0].t != NormalAddressType) {
				a = []Address{}
			}
			if parent != nil {
				parent = parent.parent
			}
			if parent != nil {
				head = parent.Header
			} else {
				head = nil
			}
		}
		if len(a) == 0 {
			// if there is an X-From-Line, it could be old damaged
			// gnus mail, fcc'd before a From line was added. Let's
			// try.
			for _, f := range h.Fields {
				if f.Name() == "X-From-Line" {
					ap := NewAddressParser(section(f.rfc822(false), " ", 1))
					ap.assertSingleAddress()
					if ap.firstError == nil {
						a = ap.Addresses
					}
					break
				}
			}
		}
		if len(a) > 0 {
			h.Add(FromFieldName, a[0].toString(false))
		}
	}

	// Some spammers like to get return receipts while hiding their
	// Fromness, so if From is bad and either Return-Receipt-To or
	// Disposition-Notification-To is good, use those.
	if h.mode == RFC5322Header &&
		(h.field(FromFieldName, 0) == nil ||
			!h.field(FromFieldName, 0).Valid() &&
				len(h.Addresses(FromFieldName)) == 0) {
		a := []Address{}
		for _, f := range h.Fields {
			if f.Name() == "Return-Receipt-To" ||
				f.Name() == "Disposition-Notification-To" {
				ap := NewAddressParser(section(f.rfc822(false), " ", 1))
				ap.assertSingleAddress()
				if ap.firstError == nil {
					a = ap.Addresses
				}
			}
			if len(a) > 0 {
				break
			}
		}
		if len(a) > 0 {
			h.RemoveAllNamed(FromFieldName)
			h.Add(FromFieldName, a[0].toString(false))
		}
	}

	// If there is an unacceptable Received field somewhere, remove it and all
	// the older Received fields.

	if occurrences[ReceivedFieldName] > 0 {
		bad := false
		i := 0
		for i < len(h.Fields) {
			if h.Fields[i].Name() == ReceivedFieldName {
				if !h.Valid() {
					bad = true
				}
				if bad {
					h.RemoveAt(i)
				} else {
					i++
				}
			} else {
				i++
			}
		}
	}

	// For some header fields which can contain errors, our best
	// option is to remove them. A field belongs here if it can be
	// parsed somehow and can be dropped without changing the meaning
	// of the rest of the message.

	if occurrences[ContentLocationFieldName] > 0 ||
		occurrences[ContentDispositionFieldName] > 0 ||
		occurrences[ContentIDFieldName] > 0 ||
		occurrences[MessageIDFieldName] > 0 {
		i := 0
		for i < len(h.Fields) {
			if (h.Fields[i].Name() == ContentLocationFieldName ||
				h.Fields[i].Name() == ContentDispositionFieldName ||
				h.Fields[i].Name() == ContentIDFieldName ||
				h.Fields[i].Name() == MessageIDFieldName) &&
				!h.Fields[i].Valid() {
				h.RemoveAt(i)
			} else {
				i++
			}
		}
	}

	// If there's more than one Sender field, preserve the first that
	// a) is syntactically valid and b) is different from From, and
	// remove the others.

	if occurrences[SenderFieldName] > 1 {
		var good *AddressField
		from := h.addressField(FromFieldName, 0)
		for _, f := range h.Fields {
			if f.Name() == SenderFieldName {
				if f.Valid() && good == nil {
					candidate := f.(*AddressField)
					if !sameAddresses(candidate, from) {
						good = candidate
					}
				}
			}
			if good != nil {
				break
			}
		}
		if good != nil {
			i := 0
			for i < len(h.Fields) {
				if h.Fields[i].Name() == SenderFieldName && h.Fields[i] != good {
					h.RemoveAt(i)
				} else {
					i++
				}
			}
		}
	}

	// Various spammers send two subject fields, and the resulting
	// rejection drag down our parse scores. But we can handle these:
	// - if one field is unparsable and the other is not, take the
	//   parsable one
	// - if one field is very long, it's bad
	// - if one field is long and contains other header field names,
	//   it's bad
	// - otherwise, the first field comes from the exploited software
	//   and the second from the exploiting.

	if occurrences[SubjectFieldName] > 1 {
		bad := []Field{}
		for _, s := range h.Fields {
			if s.Name() == SubjectFieldName {
				v := s.Value()
				b := false
				if len(v) > 300 {
					b = true
				} else if len(v) > 80 {
					v = simplify(v)
					for _, w := range strings.Split(v, " ") {
						if strings.HasSuffix(w, ":") && isAscii(w) && isKnownField[w[:len(w)-1]] {
							b = true
						}
						if b {
							break
						}
					}
				} else {
					i := 0
					for i < len(v) && v[i] < 128 {
						i++
					}
					if i < len(v) {
						b = true
					}
				}
				if b {
					bad = append(bad, s)
				}
			}
		}
		if len(bad) < occurrences[SubjectFieldName] {
			for _, b := range bad {
				h.Remove(b)
			}
			i := 0
			seen := false
			for i < len(h.Fields) {
				s := h.Fields[i]
				if s.Name() == SubjectFieldName {
					if seen {
						h.RemoveAt(i)
					} else {
						i++
					}
				} else {
					i++
				}
			}
		}
	}

	// If it's a multipart and the c-t field could not be parsed, try
	// to find the boundary by inspecting the body.

	if occurrences[ContentTypeFieldName] > 0 && body != "" {
		ct := h.ContentType()
		if !ct.Valid() &&
			ct.Type == "multipart" &&
			ct.parameter("boundary") == "" {
			cand := 0
			for body[cand] == '\n' {
				cand++
			}
			confused := false
			done := false
			boundary := ""
			for cand >= 0 && cand < len(body) && !done && !confused {
				if len(body) > cand+1 && body[cand] == '-' && body[cand+1] == '-' {
					i := cand + 2
					c := body[i]
					// bchars := bcharsnospace / " "
					// bcharsnospace := DIGIT / ALPHA / "'" / "(" / ")" /
					//                  "+" / "_" / "," / "-" / "." /
					//                  "/" / ":" / "=" / "?"
					for (c >= 'a' && c <= 'z') ||
						(c >= 'A' && c <= 'Z') ||
						(c >= '0' && c <= '9') ||
						c == '\'' || c == '(' || c == ')' ||
						c == '+' || c == '_' || c == ',' ||
						c == '-' || c == '.' || c == '/' ||
						c == ':' || c == '=' || c == '?' ||
						c == ' ' {
						i++
						c = body[i]
					}
					if i > cand+2 &&
						(body[i] == '\r' || body[i] == '\n') {
						// found a candidate line.
						s := body[cand+2 : i]
						if boundary == "" {
							boundary = s
						} else if boundary == s {
							// another boundary, fine
						} else if len(s) == len(boundary)+2 &&
							strings.HasPrefix(s, boundary) &&
							strings.HasSuffix(s, "--") {
							// it's the end boundary
							done = true
						} else if len(s) <= 70 {
							// we've seen different boundary lines. oops.
							confused = true
						}
					}
				}
				prior := cand + 1
				cand = strings.Index(body[prior:], "\n--")
				if cand >= 0 {
					cand += prior
					cand++
				}
			}
			if boundary != "" && !confused {
				ct.addParameter("boundary", boundary)
				ct.err = nil // may override other errors. ok.
			}
		}
	}

	// If the From field is syntactically invalid, but we could parse
	// one or more good addresses, kill the bad one(s) and go ahead.

	if occurrences[FromFieldName] == 1 {
		from := h.addressField(FromFieldName, 0)
		if !from.Valid() {
			good := []Address{}
			for _, a := range from.Addresses {
				if a.err == nil && a.t == NormalAddressType && a.localpartIsSensible() {
					good = append(good, a)
				}
			}
			if len(good) > 0 {
				from.Addresses = good
				from.err = nil
			}
		}
	}

	// If the from field is bad, but there is a good sender or
	// return-path, copy s/rp into from.

	if occurrences[FromFieldName] == 1 &&
		(occurrences[SenderFieldName] == 1 ||
			occurrences[ReturnPathFieldName] == 1) {
		from := h.addressField(FromFieldName, 0)
		if !from.Valid() {
			// XXX we only consider s/rp good if the received chain is
			// unbroken. This is a proxy test: We should really be
			// checking for a pure-smtp received chain and abort if
			// there are any imap/pop/http/other hops.
			seenReceived := false
			seenOther := false
			unbrokenReceived := true
			for _, f := range h.Fields {
				if f.Name() == ReceivedFieldName {
					if seenOther {
						unbrokenReceived = false // rcvd, other, then rcvd
					} else {
						seenReceived = true // true on first received
					}
				} else {
					if seenReceived {
						seenOther = true // true on first other after rcvd
					}
				}
				if !unbrokenReceived {
					break
				}
			}
			if unbrokenReceived {
				rp := h.addressField(ReturnPathFieldName, 0)
				sender := h.addressField(SenderFieldName, 0)
				var a *Address
				if rp != nil && rp.Valid() {
					l := rp.Addresses
					if len(l) > 0 && l[0].t != BounceAddressType {
						a = &l[0]
					}
				}
				if a == nil && sender != nil && sender.Valid() {
					l := sender.Addresses
					if len(l) > 0 && l[0].t != BounceAddressType {
						a = &l[0]
					}
				}
				if a != nil {
					from.Addresses = []Address{*a}
					from.err = nil
				}
			}
		}
	}

	// If there are two content-type fields, one is text/plain, and
	// the other is something other than text/plain and text/html,
	// then drop the text/plain one. It's frequently added as a
	// default, sometimes by software which doesn't check thoroughly.
	if occurrences[ContentTypeFieldName] == 2 {
		plain := false
		html := false
		n := 0
		var keep *ContentType
		for n < 2 {
			f := h.field(ContentTypeFieldName, n).(*ContentType)
			if f.Type == "text" && f.Subtype == "plain" {
				plain = true
			} else if f.Type == "text" && f.Subtype == "html" {
				html = true
			} else {
				keep = f
			}
			n++
		}
		if plain && !html && keep != nil {
			i := 0
			for i < len(h.Fields) {
				if h.Fields[i].Name() == ContentTypeFieldName &&
					h.Fields[i] != keep {
					h.RemoveAt(i)
				} else {
					i++
				}
			}
		}
	}

	// If there are several Content-Type fields, we can classify them
	// as good, bad and neutral.
	// - Good multiparts have a boundary and it occurs
	// - Good HTML starts with doctype or html
	// - Syntactically invalid fields are bad
	// - All others are neutral
	// If we have at least one good field at the end, we dump the
	// neutral and bad ones. If we have no good fields, one neutral
	// field and the rest bad, we dump the bad ones.

	if occurrences[ContentTypeFieldName] > 1 {
		good := []*ContentType{}
		bad := []*ContentType{}
		neutral := []*ContentType{}
		i := 0
		hf := h.field(ContentTypeFieldName, 0)
		for hf != nil {
			ct := hf.(*ContentType)
			if !hf.Valid() {
				bad = append(bad, ct)
			} else if ct.Type == "text" && ct.Subtype == "html" {
				l := len(body)
				if l > 2048 {
					l = 2048
				}
				b := strings.ToLower(simplify(body[:l]))
				if strings.HasPrefix(b, "<!doctype") ||
					strings.HasPrefix(b, "<html") {
					good = append(good, ct)
				} else {
					bad = append(bad, ct)
				}
			} else if ct.Type == "multipart" {
				b := ct.parameter("boundary")
				if b == "" || b != simplify(b) {
					bad = append(bad, ct)
				} else if strings.HasPrefix(body, "\n--"+b) ||
					strings.Contains(body, "\n--"+b) {
					good = append(good, ct)
				} else {
					bad = append(bad, ct)
				}
			} else {
				neutral = append(neutral, ct)
			}
			i++
			hf = h.field(ContentTypeFieldName, i)
		}
		if len(good) > 0 {
			h.RemoveAllNamed(ContentTypeFieldName)
			h.addField(good[0])
		} else if len(neutral) == 1 {
			h.RemoveAllNamed(ContentTypeFieldName)
			h.addField(neutral[0])
		}
	}

	// If there are several content-type fields, all text/html, and
	// they're different, we just remove all but one. Why are webheads
	// so clueless?

	if occurrences[ContentTypeFieldName] > 1 {
		ct := h.ContentType()
		i := 1
		for ct != nil && ct.Valid() && ct.Type == "text" && ct.Subtype == "html" {
			hf := h.field(ContentTypeFieldName, i)
			ct, _ = hf.(*ContentType)
			i++
		}
		if ct == nil {
			ct = h.ContentType()
			h.RemoveAllNamed(ContentTypeFieldName)
			h.addField(ct)
		}
	}

	// If Sender contains more than one address, that may be due to
	// inappropriate fixups. For example, javamail+postfix will create
	// Sender: System@postfix, Administrator@postfix, root@origin
	//
	// We can fix that: if all addresses but the last have the same
	// domain, and the last has a different domain, drop the first
	// ones. There are also other possible algorithms.

	if len(h.Addresses(SenderFieldName)) > 1 {
		sender := h.addressField(SenderFieldName, 0)
		domain := strings.ToTitle(sender.Addresses[0].Domain)
		i := 0
		for i < len(sender.Addresses) && strings.ToTitle(sender.Addresses[i].Domain) == domain {
			i++
		}
		if i == len(sender.Addresses)-1 {
			sender.Addresses = sender.Addresses[len(sender.Addresses)-1:]
			sender.err = nil
		}
	}

	// Some crapware tries to send DSNs without a From field. We try
	// to patch it up. We don't care very much, so this parses the
	// body and discards the result, does a _very_ quick job of
	// parsing message/delivery-status, doesn't handle xtext, and
	// doesn't care whether it uses Original-Recipient or
	// Final-Recipient.
	if h.mode == RFC5322Header &&
		(h.field(FromFieldName, 0) == nil ||
			h.field(FromFieldName, 0).Error() != nil &&
				strings.Contains(h.field(FromFieldName, 0).Error().Error(), "No-bounce")) &&
		h.ContentType() != nil &&
		h.ContentType().Type == "multipart" &&
		h.ContentType().Subtype == "report" &&
		h.ContentType().parameter("report-type") == "delivery-status" {
		ct := h.ContentType()
		tmp := &Part{}
		tmp.parseMultipart(body, ct.parameter("boundary"), false)
		for _, p := range tmp.Parts {
			h := p.Header
			var ct *ContentType
			if h != nil {
				ct = h.ContentType()
			}
			if ct != nil && ct.Type == "message" && ct.Subtype == "delivery-status" {
				// woo.
				lines := strings.Split(p.Data, "\n")
				reportingMta := ""
				var address *Address
				for _, l := range lines {
					line := strings.ToLower(l)
					field := simplify(section(line, ":", 1))
					domain := simplify(section(section(line, ":", 2), ";", 1))
					value := simplify(section(section(line, ":", 2), ";", 2))
					// value may be xtext, but I don't care. it's an
					// odd error case in illegal mail, so who can say
					// that the sender knows the xtext rules anyway?
					if field == "reporting-mta" && domain == "dns" &&
						value != "" {
						reportingMta = value
					} else if (field == "final-recipient" ||
						field == "original-recipient") &&
						domain == "rfc822" &&
						address == nil && value != "" {
						ap := NewAddressParser(value)
						for _, a := range ap.Addresses {
							if a.err == nil && a.Domain != "" {
								address = &a
								break
							}
						}
					}
				}
				if reportingMta != "" && address != nil {
					name, _ := decode(reportingMta, "us-ascii")
					name += " postmaster"
					postmaster := NewAddress(name, "postmaster", strings.ToLower(address.Domain))
					from := h.addressField(FromFieldName, 0)
					if from != nil {
						from.err = nil
						from.Addresses = []Address{}
					} else {
						from = NewAddressField(FromFieldName)
						h.addField(from)
					}
					from.Addresses = append(from.Addresses, postmaster)
					break
				}
			}
		}
	}

	// If the From field is the bounce address, and we still haven't
	// salvaged it, and the message-id wasn't added here, we use
	// postmaster@<message-id-domain> and hope the postmaster there
	// knows something about the real origin.

	if occurrences[FromFieldName] == 1 &&
		occurrences[MessageIDFieldName] == 1 {
		from := h.addressField(FromFieldName, 0)
		if !from.Valid() {
			l := from.Addresses
			if len(l) == 1 && l[0].t == BounceAddressType {
				var msgid *Address
				al := h.Addresses(MessageIDFieldName)
				if len(al) > 0 {
					msgid = &al[0]
				}

				Hostname := "localhost" // FIXME: this should be configurable
				me := strings.ToLower(Hostname)
				victim := ""
				if msgid != nil {
					victim = strings.ToLower(msgid.Domain)
				}
				tld := len(victim)
				if victim[tld-3] == '.' {
					tld -= 3 // .de
				} else if victim[tld-4] == '.' {
					tld -= 4 // .com
				}
				if tld < len(victim) {
					if victim[tld-3] == '.' {
						tld -= 3 // .co.uk
					} else if victim[tld-4] == '.' {
						tld -= 4 // .com.au
					} else if tld == len(victim)-2 && victim[tld-5] == '.' {
						tld -= 5 // .priv.no
					}
				}
				dot := strings.Index(victim, ".")
				if dot < tld {
					victim = victim[dot+1:]
					tld -= dot + 1
				}
				if victim != "" &&
					victim != me && !strings.HasSuffix(me, "."+victim) &&
					tld < len(victim) {
					replacement := NewAddress("postmaster (on behalf of unnamed "+msgid.Domain+" user)", "postmaster", victim)
					from.Addresses = []Address{replacement}
					from.err = nil
				}
			}
		}
	}

	// If we have NO From field, or one which contains only <>, use
	// invalid@invalid.invalid. We try to include a display-name if we
	// can find one. hackish hacks abound.
	if h.mode == RFC5322Header &&
		(h.field(FromFieldName, 0) == nil ||
			(!h.field(FromFieldName, 0).Valid() &&
				h.Addresses(FromFieldName) == nil) ||
			h.field(FromFieldName, 0).Error() != nil &&
				strings.Contains(h.field(FromFieldName, 0).Error().Error(), "No-bounce")) {
		from := h.addressField(FromFieldName, 0)
		raw := ""
		if from != nil {
			raw = simplify(from.UnparsedValue())
		}
		if strings.HasSuffix(raw, "<>") {
			raw = simplify(raw[:len(raw)-2])
		}
		if strings.HasPrefix(raw, "\"\"") {
			raw = simplify(raw[2:])
		}
		if strings.HasPrefix(raw, "\" \"") {
			raw = simplify(raw[3:])
		}
		if strings.Contains(raw, "<") && strings.Index(raw, "<") > 3 {
			raw = section(raw, "<", 1)
		}
		if strings.HasPrefix(raw, "\"") && strings.Index(raw[1:], "\"")+1 > 2 {
			raw = section(raw, "\"", 2) // "foo"bar > foo
		}
		raw = simplify(unquote(unquote(raw, '"', '\\'), '\'', '\\'))
		lt := strings.Index(raw, "<")
		if strings.Contains(raw, "<") &&
			strings.Index(raw[1+lt:], ">")+1+lt > 2+lt {
			raw = simplify(section(section(raw, "<", 2), ">", 1))
		}
		if strings.HasPrefix(raw, "<") && strings.HasSuffix(raw, ">") {
			raw = simplify(raw[1 : len(raw)-1])
		}
		if len(raw) < 3 {
			raw = ""
		}

		// FIXME: aox attempts to infer `raw` encoding by text analysis; falls back to ascii
		codec := "us-ascii"
		d, _ := decode(raw, codec)
		n := simplify(d)
		if n != "" {
			// look again and get rid of <>@
			i := 0
			var buf bytes.Buffer
			fffd := false
			known := 0
			for i < len(n) {
				// FIXME: removed check for 0xFFFD
				if n[i] == '@' || n[i] == '<' || n[i] == '>' ||
					n[i] < ' ' || (n[i] >= 128 && n[i] < 160) {
					fffd = true
				} else {
					if fffd && buf.Len() > 0 {
						buf.WriteRune(0xFFFD)
					}
					buf.WriteByte(n[i])
					fffd = false
					known++
				}
				i++
			}
			n = buf.String()
			if known < 3 {
				n = ""
			}
		}
		a := NewAddress(n, "invalid", "invalid.invalid")
		if from != nil {
			from.err = nil
			from.Addresses = []Address{a}
		} else {
			from = NewAddressField(FromFieldName)
			from.Addresses = append(from.Addresses, a)
			h.addField(from)
		}
	}

	// If the Reply-To field is bad and From is good, we forget
	// Reply-To entirely.

	if occurrences[FromFieldName] > 0 &&
		occurrences[ReplyToFieldName] > 0 {
		from := h.addressField(FromFieldName, 0)
		rt := h.addressField(ReplyToFieldName, 0)
		if from.Valid() && !rt.Valid() && len(from.Addresses) > 0 {
			h.RemoveAllNamed(ReplyToFieldName)
		}
	}

	// If c-t-e is bad, we try to detect.

	if occurrences[ContentTransferEncodingFieldName] > 0 {
		cte := h.ContentTransferEncoding()
		cte2 := h.field(ContentTransferEncodingFieldName, 1)
		if cte != nil && (cte2 != nil || !cte.Valid()) {
			minl := math.MaxInt32 - 1
			maxl := 0
			i := 0
			l := 0
			n := 0
			for i < len(body) {
				if body[i] == '\n' || body[i] == '\r' {
					if l > maxl {
						maxl = l
					}
					if l < minl {
						minl = l
					}
					l = 0
					n++
				} else {
					l++
				}
				i++
			}
			if n > 5 && maxl == minl && minl > 50 {
				// more than five lines, all (except the last) equally
				// long. it really looks like base64.
				h.RemoveAllNamed(ContentTransferEncodingFieldName)
				h.Add(ContentTransferEncodingFieldName, "base64")
			} else {
				// it can be q-p or none. do we really care? can we
				// even decide reliably? I think we might as well
				// assume none.
				h.RemoveAllNamed(ContentTransferEncodingFieldName)
			}
		}
	}

	// Some people don't know c-t from c-t-e

	if occurrences[ContentTransferEncodingFieldName] == 0 &&
		occurrences[ContentTypeFieldName] > 0 &&
		!h.ContentType().Valid() {
		phaps := NewContentTransferEncoding()
		phaps.Parse(h.ContentType().UnparsedValue())
		if phaps.Valid() {
			h.RemoveAllNamed(ContentTransferEncodingFieldName)
			h.RemoveAllNamed(ContentTypeFieldName)
			h.addField(phaps)
			h.Add("Content-Type", "application/octet-stream")
		}
	}

	// If Content-Base or Content-Location is/are bad, we just drop it/them

	if h.field(ContentBaseFieldName, 0) != nil || h.field(ContentLocationFieldName, 0) != nil {
		i := 0
		for i < len(h.Fields) {
			if !h.Fields[i].Valid() &&
				(h.Fields[i].Name() == ContentBaseFieldName ||
					h.Fields[i].Name() == ContentLocationFieldName) {
				h.RemoveAt(i)
			} else {
				i++
			}
		}
	}

	h.verified = false
}

// Returns the canonical text representation of this Header.  Downgrades rather
// than including UTF-8 if \a avoidUTF8 is true.
func (h *Header) AsText(avoidUTF8 bool) string {
	buf := bytes.NewBuffer(make([]byte, 0, len(h.Fields)*100))

	for _, f := range h.Fields {
		h.appendField(buf, f, avoidUTF8)
	}

	return buf.String()
}

// Appends the string representation of the field \a hf to \a r. Does nothing
// if \a f is nil.
//
// This function doesn't wrap. That's probably a bug. How to fix it?
//
// (The details of the function are liable to change.)
func (h *Header) appendField(buf *bytes.Buffer, f Field, avoidUTF8 bool) {
	if f == nil {
		return
	}

	buf.WriteString(f.Name())
	buf.WriteString(": ")
	buf.WriteString(f.rfc822(avoidUTF8))
	buf.WriteString(crlf)
}
