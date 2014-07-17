package mail

import (
	"strings"
)

type HeaderMode int

const (
	HEADER_RFC5322 HeaderMode = iota
	HEADER_MIME
)

type Header struct {
	mode   HeaderMode
	fields []Field
}

func ReadHeader(rfc5322 string, m HeaderMode) (h *Header, err error) {
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

		if j == i+4 && m == HEADER_RFC5322 && strings.ToLower(rfc5322[i:j+1]) == "from " {
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
				f := NewHeaderField(name, value)
				h.Add(f)
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

	return h, nil
}

func (h *Header) Add(f Field) {
	h.fields = append(h.fields, f)
}

func (h *Header) addressField(fn string, n int) Field {
	switch fn {
	case FromFieldName, ResentFromFieldName, SenderFieldName, ResentSenderFieldName:
		f, ok := h.field(fn, n).(*AddressField)
		if ok {
			return f
		} else {
			return nil
		}
	default:
		return nil
	}
}

func (h *Header) field(fn string, n int) Field {
	for _, field := range h.fields {
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
