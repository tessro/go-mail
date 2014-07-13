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

Header contains a HeaderData

HeaderData contains a list of HeaderField

HeaderField contains a HeaderFieldData

MimeField is a HeaderField
MimeField contains a MimeFieldData

MimeFieldData contains a list of Parameter

ContentType is a MimeField

ContentTransferEncoding is a MimeField

ContentDisposition is a MimeField

ContentLanguage is a MimeField

*/

const CRLF = "\015\012"

type Part struct {
	parent *Part

	Header Header
	Parts  []*Part
}

type Message struct {
	Rfc822Size   int
	InternalDate int
}

func ReadMessage(rfc5322 string) (m *Message, err error) {
	i := 0
	h, err := ReadHeader(rfc5322[i:], HEADER_RFC5322)
	for _, f := range h.fields {
		log.Printf("header: %s = %q", f.Name(), f.Value())
	}
	return m, nil
}
