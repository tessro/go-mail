package mail

import (
	"log"
)

/*

Message is a Multipart
Message contains a MessageData

Multipart contains a Header
Multipart contains a Multipart (parent)
Multipart contains a list of Bodypart (children)

Bodypart is a Multipart
Bodypart contains a BodypartData

BodypartData contains data (raw body?)
BodypartData contains text (processed body?)
BodypartData contains Message (in case this is a message/rfc822 embedded part; holds metadata)

*/

const CRLF = "\015\012"

type Part struct {
	parent *Part

	Header *Header
	Parts  []*Part
}

type Message struct {
	Part
	Rfc822Size   int
	InternalDate int
}

func ReadMessage(rfc5322 string) (m *Message, err error) {
	i := 0
	h, err := ReadHeader(rfc5322[i:], HEADER_RFC5322)
	if err != nil {
		return nil, err
	}
	m = &Message{}
	m.Header = h

	ct := h.ContentType()
	if ct != nil && ct.Type == "multipart" {
	} else {
	}

	log.Printf("ct: %s/%s %+v", ct.Type, ct.Subtype, ct.Parameters)
	return m, nil
}
