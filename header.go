package mail

import (
	"log"
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
			log.Printf("name = %q", name)
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
			log.Printf("value = %q", value)
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
