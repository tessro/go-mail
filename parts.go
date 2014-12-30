package mail

import (
	"bytes"
	"errors"
	"strings"

	"github.com/paulrosania/go-charset/charset"
)

type Part struct {
	message *Message
	parent  *Part

	Header *Header
	Parts  []*Part
	Number int

	HasText bool
	Text    string
	Data    string

	numBytes        int
	numEncodedBytes int
	numEncodedLines int

	err error
}

// Appends the text of this multipart MIME entity to the buffer \a buf.
func (p *Part) appendMultipart(buf *bytes.Buffer, avoidUtf8 bool) {
	ct := p.Header.ContentType() // <<< I think this is the only reference to p during marshaling
	delim := ct.parameter("boundary")
	buf.WriteString("--" + delim)
	for _, c := range p.Parts {
		buf.WriteString(CRLF)

		buf.WriteString(c.Header.AsText(avoidUtf8))
		buf.WriteString(CRLF)
		p.appendAnyPart(buf, c, ct, avoidUtf8)

		buf.WriteString(CRLF)
		buf.WriteString("--")
		buf.WriteString(delim)
	}
	buf.WriteString("--")
	buf.WriteString(CRLF)
}

// This function appends the text of the MIME bodypart \a bp with Content-Type
// \a ct to the buffer \a buf.
//
// The details of this function are certain to change.
func (p *Part) appendAnyPart(buf *bytes.Buffer, bp *Part, ct *ContentType, avoidUtf8 bool) {
	childct := bp.Header.ContentType()
	e := BinaryEncoding
	cte := bp.Header.ContentTransferEncoding()
	if cte != nil {
		e = cte.Encoding
	}

	if (childct != nil && childct.Type == "message") ||
		(ct != nil && ct.Type == "multipart" && ct.Subtype == "digest" && childct == nil) {
		if childct != nil && childct.Subtype != "rfc822" {
			p.appendTextPart(buf, bp, childct)
		} else {
			buf.WriteString(bp.message.Rfc822(avoidUtf8))
		}
	} else if childct == nil || strings.ToLower(childct.Type) == "text" {
		p.appendTextPart(buf, bp, childct)
	} else if childct.Type == "multipart" {
		bp.appendMultipart(buf, avoidUtf8)
	} else {
		buf.WriteString(encodeCTE(bp.Data, e, 72))
	}
}

// This function appends the text of the MIME bodypart \a bp with Content-Type
// \a ct to the buffer \a buf.
//
// The details of this function are certain to change.
func (p *Part) appendTextPart(buf *bytes.Buffer, bp *Part, ct *ContentType) {
	e := BinaryEncoding
	cte := bp.Header.ContentTransferEncoding()
	if cte != nil {
		e = cte.Encoding
	}

	var c *charset.Charset
	if ct != nil && ct.parameter("charset") != "" {
		c = charset.Info(ct.parameter("charset"))
	}
	if c == nil {
		// TODO: infer encoding from text
	}

	// TODO: encode into original charset
	body := bp.Text

	buf.WriteString(encodeCTE(body, e, 72))
}

// Returns the text representation of this Bodypart.
//
// Notes: This function seems uncomfortable. It returns just one of many
// possible text representations, and the exact choice seems arbitrary, and
// finally, it does rather overlap with text() and data().
//
// We probably should transition away from this function.
//
// The exact representation returned uses base64 encoding for data types and no
// ContentTransferEncoding. For text types, it encodes the text according to
// the ContentType.
func (p *Part) AsText(avoidUtf8 bool) string {
	r := ""
	var c *charset.Charset

	ct := p.Header.ContentType()
	if ct != nil && ct.parameter("charset") != "" {
		c = charset.Info(ct.parameter("charset"))
	}
	if c == nil {
		c = charset.Info("us-ascii")
	}

	if len(p.Parts) > 0 {
		buf := bytes.NewBuffer(make([]byte, 0))
		p.appendMultipart(buf, avoidUtf8)
		r = buf.String()
	} else if p.Header.ContentType() == nil ||
		p.Header.ContentType().Type == "text" {
		r, _ = decode(p.Text, c.Name)
	} else {
		r = e64(p.Data, 72)
	}

	return r
}

// Parses the part of \a rfc2822 from index \a i to (but not including) \a end,
// dividing the part into bodyparts wherever the boundary \a divider occurs and
// adding each bodypart to \a children, and setting the correct \a parent. \a
// divider does not contain the leading or trailing hyphens. \a digest is true
// for multipart/digest and false for other types.
func (p *Part) parseMultipart(rfc5322, divider string, digest bool) {
	i := 0
	start := 0
	last := false
	pn := 1
	end := len(rfc5322)
	for !last && i <= end {
		if i >= end ||
			rfc5322[i] == '-' && rfc5322[i+1] == '-' &&
				(i == 0 || rfc5322[i-1] == 31 || rfc5322[i-1] == 10) &&
				rfc5322[i+2] == divider[0] &&
				rfc5322[i+2:i+2+len(divider)] == divider {
			j := i
			l := false
			if i >= end {
				l = true
			} else {
				j = i + 2 + len(divider)
				if rfc5322[j] == '-' && rfc5322[j+1] == '-' {
					j += 2
					l = true
				}
			}
			for rfc5322[j] == ' ' || rfc5322[j] == '\t' {
				j++
			}
			if rfc5322[j] == 13 || rfc5322[j] == 10 || j >= len(rfc5322) {
				// finally. we accept that as a boundary line.
				if j < len(rfc5322) && rfc5322[j] == 13 {
					j++
				}
				if j < len(rfc5322) && rfc5322[j] == 10 {
					j++
				}
				if start > 0 {
					h, _ := ReadHeader(rfc5322[start:j], MimeHeader)
					start += h.numBytes
					if digest {
						h.DefaultType = MessageRfc822ContentType
					}

					h.Repair()

					// Strip the [CR]LF that belongs to the boundary.
					if rfc5322[i-1] == 10 {
						i--
						if rfc5322[i-1] == 13 {
							i--
						}
					}

					bp := p.parseBodypart(rfc5322[start:i], h)
					bp.Number = pn
					p.Parts = append(p.Parts, bp)
					pn++

					h.RepairWithBody(bp, "")
				}
				last = l
				start = j
				i = j
			}
		}
		for i < end && rfc5322[i] != 13 && rfc5322[i] != 10 {
			i++
		}
		for i < end && (rfc5322[i] == 13 || rfc5322[i] == 10) {
			i++
		}
	}
}

func guessTextCodec(body string) *charset.Charset {
	// step 1. try iso-2022-jp. this goes first because it's so
	// restrictive, and because 2022 strings also match the ascii and
	// utf-8 tests.
	if body[0] == 0x1B &&
		(body[1] == '(' || body[1] == '$') &&
		(body[2] == 'B' || body[2] == 'J' || body[2] == '@') {
		_, err := decode(body, "iso-2022-jp")
		if err != nil {
			return charset.Info("iso-2022-jp")
		}
	}

	// step 2. could it be pure ascii?
	_, err := decode(body, "us-ascii")
	if err != nil {
		return charset.Info("us-ascii")
	}

	// some multibyte encodings have to go before utf-8, or else utf-8
	// will match. this applies at least to iso-2002-jp, but may also
	// apply to other encodings that use octet values 0x01-0x07f
	// exclusively.

	// step 3. does it look good as utf-8?
	_, err = decode(body, "utf8")
	if err != nil {
		// FIXME: skipped a check for ascii
		return charset.Info("utf8")
	}

	// step 4. guess a codec based on the bodypart content.
	// TODO: implement codec guesser

	// step 5. is utf-8 at all plausible?
	// FIXME: not reachable since we don't yet discriminate between valid and well-formed
	if err != nil {
		return charset.Info("utf8")
	}
	// should we use g here if valid()?

	return nil
}

func guessHtmlCodec(body string) *charset.Charset {
	// Let's see if the general function has something for us.
	guess := guessTextCodec(body)

	// HTML prescribes that 8859-1 is the default. Let's see if 8859-1 works.
	if guess == nil {
		_, err := decode(body, "iso-8859-1")
		if err == nil {
			guess = charset.Info("iso-8859-1")
		}
	}

	if guess == nil {
		// Some people believe that Windows codepage 1252 is
		// ISO-8859-1. Let's see if that works.
		_, err := decode(body, "cp-1252")
		if err == nil {
			guess = charset.Info("cp-1252")
		}
	}

	// Some user-agents add a <meta http-equiv="content-type"> instead
	// of the Content-Type field. Maybe that exists? And if it exists,
	// is it more likely to be correct than our guess above?

	b := simplify(strings.ToLower(body))
	i := 0
	for {
		tag := "<meta http-equiv=\"content-type\" content=\""
		next := strings.Index(b[i:], tag)
		if next < 0 {
			break
		}

		i += next
		i += len(tag)
		j := i
		for j < len(b) && b[j] != '"' {
			j++
		}
		hf := NewHeaderField("Content-Type", b[i:j])
		cs := hf.(*ContentType).parameter("charset")
		var meta *charset.Charset
		if cs != "" {
			meta = charset.Info(cs)
		}
		m := ""
		g := ""
		var merr, gerr error
		if meta != nil {
			m, merr = decode(body, meta.Name)
		}
		if guess != nil {
			g, gerr = decode(body, guess.Name)
		}
		ub, _ := decode(b, meta.Name)
		if meta != nil &&
			((m != "" && m == g) ||
				(merr == nil &&
					(guess == nil || gerr != nil)) ||
				(merr == nil && guess == nil) ||
				(merr == nil && guess != nil && guess.Name == "iso-8859-1") ||
				(merr == nil && guess != nil && gerr != nil)) &&
			strings.Contains(ascii(ub), tag) {
			guess = meta
		}
	}

	return guess
}

// Parses the part of \a rfc2822 from \a start to \a end (not including \a end)
// as a single bodypart with MIME/RFC 822 header \a h.
//
// This removes the "charset" argument from the Content-Type field in \a h.
//
// The \a parent argument is provided so that nested message/rfc822 bodyparts
// without a Date field may be fixed with reference to the Date field in the
// enclosing bodypart.
func (p *Part) parseBodypart(rfc5322 string, h *Header) *Part {
	start := 0
	end := len(rfc5322)
	if rfc5322[start] == 13 {
		start++
	}
	if rfc5322[start] == 10 {
		start++
	}

	bp := &Part{
		parent: p,
		Header: h,
	}

	body := ""
	if end > start {
		body = rfc5322
	}
	if !strings.Contains(body, "=") {
		// sometimes people send c-t-e: q-p _and_ c-t-e: 7bit or 8bit.
		// if they are equivalent we can accept it.
		i := 0
		any := false
		f := h.field(ContentTransferEncodingFieldName, i)
		for f != nil {
			if f.(*ContentTransferEncoding).Encoding == QPEncoding {
				any = true
			}
			i++
			f = h.field(ContentTransferEncodingFieldName, i)
		}
		if any && i > 1 {
			h.Fields.RemoveAllNamed(ContentTransferEncodingFieldName)
		}
	}

	e := BinaryEncoding
	cte := h.ContentTransferEncoding()
	if cte != nil {
		e = cte.Encoding
	}
	if body != "" {
		if e == Base64Encoding || e == UuencodeEncoding {
			body = decodeCTE(body, e)
		} else {
			body = decodeCTE(crlf(body), e)
		}
	}

	ct := h.ContentType()
	if ct == nil {
		switch h.DefaultType {
		case TextPlainContentType:
			h.Add(NewHeaderField("Content-Type", "text/plain"))
		case MessageRfc822ContentType:
			h.Add(NewHeaderField("Content-Type", "message/rfc822"))
		}
		ct = h.ContentType()
	}
	if ct.Type == "text" {
		specified := false
		unknown := false
		var c *charset.Charset

		if ct != nil {
			csn := ct.parameter("charset")
			if strings.ToLower(csn) == "default" {
				csn = ""
			}
			if csn != "" {
				specified = true
			}
			c = charset.Info(csn)
			if c == nil {
				unknown = true
			}
			if c != nil && strings.ToLower(c.Name) == "us-ascii" {
				// Some MTAs appear to say this in case there is no
				// Content-Type field - without checking whether the
				// body actually is ASCII. If it isn't, we'd better
				// call our charset guesser.
				_, err := decode(body, c.Name)
				if err != nil {
					specified = false
				}
			}
		}

		if c == nil {
			c = charset.Info("us-ascii")
		}

		bp.HasText = true
		t, decodeErr := decode(crlf(body), c.Name)
		bp.Text = t

		if c.Name == "GB2312" || c.Name == "ISO-2022-JP" ||
			c.Name == "KS_C_5601-1987" {
			// undefined code point usage in GB2312 spam is much too
			// common. (GB2312 spam is much too common, but that's
			// another matter.) Gb2312Codec turns all undefined code
			// points into U+FFFD, so here, we can take the unicode
			// form and say it's the canonical form. when a client
			// later reads the message, it gets the text in unicode,
			// including U+FFFD.

			bad := decodeErr != nil

			// the header may contain some unencoded gb2312. we bang
			// it by hand, ignoring errors.
			for _, f := range h.Fields {
				if !f.Valid() && f.Name() == SubjectFieldName {
					hf, ok := f.(*HeaderField)
					if ok {
						// is it right to bang only Subject?
						hf.value, decodeErr = decode(hf.UnparsedValue(), c.Name)
					}
				}
			}

			// if the body was bad, we prefer the (unicode) in
			// bp->d->text and pretend it arrived as UTF-8:
			if bad {
				body = bp.Text
			}
		}

		if (!specified && (decodeErr != nil || ct.Subtype == "html")) ||
			(specified && decodeErr != nil) {
			var g *charset.Charset
			if ct.Subtype == "html" {
				g = guessHtmlCodec(body)
			} else {
				g = guessTextCodec(body)
			}
			guessed := ""
			var gerr error
			if g != nil {
				guessed, gerr = decode(crlf(body), g.Name)
			}
			if g == nil {
				// if we couldn't guess anything, keep what we had if
				// it's valid or explicitly specified, else use
				// unknown-8bit.
				if !specified && decodeErr != nil {
					bp.Text, _ = decode(crlf(body), "unknown-8bit")
				}
			} else {
				// if we could guess something, is our guess better than what
				// we had?
				if gerr == nil && decodeErr != nil {
					c = g
					bp.Text = guessed
				}
			}
		}

		// FIXME: codec state probably matters here and we ignored it (aox cares)
		if specified && decodeErr != nil {
			// the codec was specified, and the specified codec
			// resulted in an error, but did not abort conversion. we
			// respond by forgetting the error, using the conversion
			// result (probably including one or more U+FFFD) and
			// labelling the message as UTF-8.
			body = bp.Text
		} else if !specified && decodeErr != nil {
			// the codec was not specified, and we couldn't find
			// anything. we call it unknown-8bit.
			bp.Text, _ = decode(body, "unknown-8bit")
		}

		// if we ended up using a 16-bit codec and were using q-p, we
		// need to reevaluate without any trailing CRLF
		if e == QPEncoding && strings.HasPrefix(c.Name, "UTF-16") {
			bp.Text, _ = decode(stripCRLF(body), c.Name)
		}

		if decodeErr != nil && bp.err == nil {
			errmsg := "Could not convert body to Unicode"
			if specified {
				cs := ""
				if ct != nil {
					cs = ct.parameter("charset")
				}
				if cs == "" {
					cs = c.Name
				}
				errmsg += " from " + cs
			}
			if specified && unknown {
				errmsg += ": Character set not implemented"
			} else if decodeErr != nil {
				errmsg += ": " + decodeErr.Error()
			}
			bp.err = errors.New(errmsg)
		}

		if strings.ToLower(c.Name) != "us-ascii" {
			ct.addParameter("charset", strings.ToLower(c.Name))
		} else if ct != nil {
			ct.removeParameter("charset")
		}

		body, _ = decode(bp.Text, c.Name)
		qp := needsQP(body)

		if cte != nil {
			if !qp {
				h.Fields.RemoveAllNamed(ContentTransferEncodingFieldName)
				cte = nil
			} else if cte.Encoding != QPEncoding {
				cte.Encoding = QPEncoding
			}
		} else if qp {
			h.Add(NewHeaderField("Content-Transfer-Encoding", "quoted-printable"))
			cte = h.ContentTransferEncoding()
		}
	} else {
		bp.Data = body
		if ct.Type != "multipart" && ct.Type != "message" {
			e := Base64Encoding
			// there may be exceptions. cases where some format really
			// needs another content-transfer-encoding:
			if ct.Type == "application" &&
				strings.HasPrefix(ct.Subtype, "pgp-") &&
				!needsQP(body) {
				// seems some PGP things need "Version: 1" unencoded
				e = BinaryEncoding
			} else if ct.Type == "application" && ct.Subtype == "octet-stream" &&
				strings.Contains(body, "BEGIN PGP MESSAGE") {
				// mutt cannot handle PGP in base64 (what a crock)
				e = BinaryEncoding
			}
			// change c-t-e to match the encoding decided above
			if e == BinaryEncoding {
				h.Fields.RemoveAllNamed(ContentTransferEncodingFieldName)
				cte = nil
			} else if cte != nil {
				cte.Encoding = e
			} else {
				h.Add(NewHeaderField("Content-Transfer-Encoding", "base64"))
				cte = h.ContentTransferEncoding()
			}
		}
	}

	if ct.Type == "multipart" {
		bp.parseMultipart(rfc5322[start:end], ct.parameter("boundary"), ct.Subtype == "digest")
	} else if ct.Type == "message" && ct.Subtype == "rfc822" {
		// There are sometimes blank lines before the message.
		for rfc5322[start] == 13 || rfc5322[start] == 10 {
			start++
		}
		m := &Message{}
		m.parent = bp
		m.Parse(rfc5322[start:end])
		for _, p := range m.Parts {
			bp.Parts = append(bp.Parts, p)
			p.parent = bp
		}
		bp.message = m
		body = m.Rfc822(false)
	}

	bp.numBytes = len(body)
	if cte != nil {
		body = encodeCTE(body, cte.Encoding, 72)
	}
	bp.numEncodedBytes = len(body)
	if bp.HasText || (ct.Type == "message" && ct.Subtype == "rfc822") {
		n := 0
		i := 0
		l := len(body)
		for i < l {
			if body[i] == '\n' {
				n++
			}
			i++
		}
		if l > 0 && body[l-1] != '\n' {
			n++
		}
		bp.numEncodedLines = n
	}

	h.Simplify()

	return bp
}
