package mail

import (
	"bytes"
	"strconv"
)

const CRLF = "\015\012"

type Message struct {
	Part
	Rfc822Size   int
	InternalDate int
}

func ReadMessage(rfc5322 string) (*Message, error) {
	m := &Message{}
	err := m.Parse(rfc5322)
	return m, err
}

func (m *Message) Parse(rfc5322 string) error {
	i := 0
	h, err := ReadHeader(rfc5322[i:], Rfc5322Header)
	if err != nil {
		return err
	}
	m.Header = h
	h.Repair()
	h.RepairWithBody(&m.Part, rfc5322[i:])

	ct := h.ContentType()
	if ct != nil && ct.Type == "multipart" {
		m.parseMultipart(rfc5322, ct.parameter("boundary"), ct.Subtype == "digest")
	} else {
		bp := m.parseBodypart(rfc5322, h)
		m.Parts = append(m.Parts, bp)
	}

	//m.fix8BitHeaderFields()
	m.Header.Simplify()

	return nil
}

// Returns the message formatted in RFC 822 (actually 2822) format.  The return
// value is a canonical expression of the message, not whatever was parsed.
//
// If \a avoidUtf8 is true, this function loses information rather than
// including UTF-8 in the result.
func (m *Message) Rfc822(avoidUtf8 bool) string {
	var buf *bytes.Buffer
	if m.Rfc822Size > 0 {
		buf = bytes.NewBuffer(make([]byte, 0, m.Rfc822Size))
	} else {
		buf = bytes.NewBuffer(make([]byte, 0, 50000))
	}

	buf.WriteString(m.Header.asText(avoidUtf8))
	buf.WriteString(CRLF)
	buf.WriteString(m.Body(avoidUtf8))

	return buf.String()
}

// Returns the text representation of the body of this message.
func (m *Message) Body(avoidUtf8 bool) string {
	buf := new(bytes.Buffer)

	ct := m.Header.ContentType()
	if ct != nil && ct.Type == "multipart" {
		m.appendMultipart(buf, avoidUtf8)
	} else {
		// FIXME: Is this the right place to restore this linkage?
		if len(m.Parts) > 0 {
			firstChild := m.Parts[0]
			firstChild.Header = m.Header
			m.appendAnyPart(buf, firstChild, ct, avoidUtf8)
		}
	}

	return buf.String()
}

// Returns a pointer to the Bodypart whose IMAP part number is \a s and
// possibly create it. Creates Bodypart objects if \a create is true. Returns
// null pointer if \a s is not valid and \a create is false.
func (m *Message) BodyPart(s string, create bool) *Part {
	b := 0
	var bp *Part
	for b < len(s) {
		e := b
		for e < len(s) && s[e] >= '0' && s[e] <= '9' {
			e++
		}
		if e < len(s) && s[e] != '.' {
			return nil
		}
		n, err := strconv.Atoi(s[b:e])
		b = e + 1
		if err != nil || n == 0 {
			return nil
		}
		cs := m.Parts
		if bp != nil {
			cs = bp.Parts
		}
		i := 0
		var c *Part
		for i, c = range cs {
			if c.number >= n {
				break
			}
		}
		if c != nil && c.number == n {
			if n == 1 && c.Header == nil {
				// it's possible that i doesn't have a header of its
				// own, and that the parent message's header functions
				// as such. link it in if that's the case.
				h := &Header{}
				if bp != nil && bp.message != nil {
					h = bp.message.Header
				}
				if h != nil && (h.ContentType() == nil ||
					h.ContentType().Type != "multipart") {
					c.Header = h
				}
			}
			bp = c
		} else if create {
			var child *Part
			if bp != nil {
				child = &Part{
					number: n,
					parent: bp,
				}
			} else {
				child = &Part{
					number: n,
					parent: &m.Part,
				}
			}
			cs = append(append(cs[:i], child), cs[i:]...)
			bp = child
		} else {
			return nil
		}
	}
	return bp
}
