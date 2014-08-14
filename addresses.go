package mail

import (
	"bytes"
	"fmt"
	"net"
	"strings"
)

type AddressType int

const (
	NormalAddressType AddressType = iota
	BounceAddressType
	EmptyGroupAddressType
	LocalAddressType
	InvalidAddressType
)

type Address struct {
	id        int
	name      string
	Localpart string
	Domain    string
	t         AddressType
	err       error
}

func NewAddress(name, localpart, domain string) Address {
	addr := Address{
		name:      name,
		Localpart: localpart,
		Domain:    domain,
		t:         InvalidAddressType,
	}
	if domain != "" {
		addr.t = NormalAddressType
	} else if localpart != "" {
		addr.t = LocalAddressType
	} else if name != "" {
		addr.t = EmptyGroupAddressType
	} else if name == "" && localpart == "" && domain == "" {
		addr.t = BounceAddressType
	}
	return addr
}

// Returns the name stored in this Address. The name is the RFC 2822
// display-part, or in case of memberless groups, the display-name of the
// group.
//
// A memberless group is stored as an Address whose Localpart() and Domain()
// are both empty.
func (a *Address) Name(avoidUtf8 bool) string {
	atom := true
	ascii := true

	i := 0
	for i < len(a.name) {
		c := a.name[i]

		// source: 2822 section 3.2.4
		if (c >= 'a' && c <= 'z') ||
			(c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') ||
			c == '!' || c == '#' || c == '$' || c == '%' ||
			c == '&' || c == '\'' || c == '*' || c == '+' ||
			c == '-' || c == '/' || c == '=' || c == '?' ||
			c == '^' || c == '_' || c == '`' || c == '{' ||
			c == '|' || c == '}' || c == '~' ||
			// extra
			c == ' ' {
			// still an atom
		} else if c >= 128 {
			ascii = false
			if avoidUtf8 {
				atom = false
			}
		} else {
			atom = false
		}

		i++
	}

	if atom || i == 0 {
		return a.name
	}

	if ascii || !avoidUtf8 {
		return quote(a.name, '"', '\\')
	}

	return encodePhrase(a.name)
}

// Returns the Localpart and Domain as a EString. Returns toString() if the
// type() isn't Normal or Local.
func (a *Address) lpdomain() string {
	var r string
	if a.t == NormalAddressType || a.t == LocalAddressType {
		if a.localpartIsSensible() {
			r = a.Localpart
		} else {
			r = quote(a.Localpart, '"', '\'')
		}
	}
	if a.t == NormalAddressType {
		r += "@" + a.Domain
	}
	if r == "" {
		r = a.toString(false)
	}
	return r
}

// Returns an RFC 2822 representation of this address. If \a avoidUtf8 is
// present and true (the default is false), toString() returns an address which
// avoids UTF-8 at all costs, even if that loses information.
func (a *Address) toString(avoidUtf8 bool) string {
	var r string
	switch a.t {
	case InvalidAddressType:
		r = ""
	case BounceAddressType:
		r = "<>"
	case EmptyGroupAddressType:
		r = a.Name(true) + ":;"
	case LocalAddressType:
		if avoidUtf8 && a.needsUnicode() {
			r = "this-address@needs-unicode.invalid"
		} else if a.localpartIsSensible() {
			r = a.Localpart
		} else {
			r = quote(a.Localpart, '"', '\'')
		}
	case NormalAddressType:
		if avoidUtf8 && a.needsUnicode() {
			r = "this-address@needs-unicode.invalid"
		} else {
			postfix := ""
			var buf bytes.Buffer
			if a.name != "" {
				buf.WriteString(a.Name(avoidUtf8))
				buf.WriteString(" <")
				postfix = ">"
			}
			if a.localpartIsSensible() {
				buf.WriteString(a.Localpart)
			} else {
				buf.WriteString(quote(a.Localpart, '"', '\''))
			}
			buf.WriteByte('@')
			buf.WriteString(a.Domain)
			buf.WriteString(postfix)
			r = buf.String()
		}
	}
	return r
}

// Returns true if this is a sensible-looking Localpart, and false if it needs
// quoting. We should never permit one of our users to need quoting, but we
// must permit foreign addresses that do.
func (a *Address) localpartIsSensible() bool {
	if a.Localpart == "" {
		return false
	}
	i := 0
	for i < len(a.Localpart) {
		c := a.Localpart[i]
		if c == '.' {
			if a.Localpart[i+1] == '.' {
				return false
			}
		} else if !((c >= 'a' && c <= 'z') ||
			(c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') ||
			c == '!' || c == '#' ||
			c == '$' || c == '%' ||
			c == '&' || c == '\'' ||
			c == '*' || c == '+' ||
			c == '-' || c == '/' ||
			c == '=' || c == '?' ||
			c == '^' || c == '_' ||
			c == '`' || c == '{' ||
			c == '|' || c == '}' ||
			c == '~' || c >= 161) {
			return false
		}
		i++
	}
	return true
}

// Returns true if this message needs unicode address support, and false if it
// can be transmitted over plain old SMTP.
//
// Note that the display-name can require unicode even if the address does not.
func (a *Address) needsUnicode() bool {
	if isAscii(a.Localpart) && isAscii(a.Domain) {
		return false
	}
	return true
}

type Addresses []Address

// The AddressParser class helps parse email addresses and lists.
//
// In the interests of simplicity, AddressParser parses everything as
// if it were a list of addresses - either of the mailbox-list or
// address-list productions in RFC 2822. The user of this class must
// check that the supplied addresses fit the (often more specific)
// requirements.
//
// AddressParser supports most of RFC 822 and 2822, but mostly omits
// address groups. An empty address group is translated into a single
// Address, a nonempty group is translated into the equivalent number
// of addresses.
//
// AddressParser does not attempt to canonicalize the addresses
// parsed or get rid of duplicates (To: ams@oryx.com, ams@ory.com),
// it only parses.
//
// The first error seen is stored and can be accessed using error().
type AddressParser struct {
	s           string
	firstError  error
	recentError error
	Addresses   Addresses
	lastComment string
}

/*
address         =       mailbox / group
mailbox         =       name-addr / addr-spec
name-addr       =       [display-name] angle-addr
angle-addr      =       [CFWS] "<" addr-spec ">" [CFWS] / obs-angle-addr
group           =       display-name ":" [mailbox-list / CFWS] ";"
                        [CFWS]
display-name    =       phrase
mailbox-list    =       (mailbox *("," mailbox)) / obs-mbox-list
address-list    =       (address *("," address)) / obs-addr-list
addr-spec       =       local-part "@" Domain
local-part      =       dot-atom / quoted-string / obs-local-part
Domain          =       dot-atom / Domain-literal / obs-Domain
Domain-literal  =       [CFWS] "[" *([FWS] dcontent) [FWS] "]" [CFWS]
dcontent        =       dtext / quoted-pair
dtext           =       NO-WS-CTL /     ; Non white space controls
                        %d33-90 /       ; The rest of the US-ASCII
                        %d94-126        ;  characters not including "[",
                                        ;  "]", or "\"
obs-angle-addr  =       [CFWS] "<" [obs-route] addr-spec ">" [CFWS]
obs-route       =       [CFWS] obs-Domain-list ":" [CFWS]
obs-Domain-list =       "@" Domain *(*(CFWS / "," ) [CFWS] "@" Domain)
obs-local-part  =       word *("." word)
obs-Domain      =       atom *("." atom)
obs-mbox-list   =       1*([mailbox] [CFWS] "," [CFWS]) [mailbox]
obs-addr-list   =       1*([address] [CFWS] "," [CFWS]) [address]
*/

// Constructs an Address Parser parsing \a s. After construction, addresses()
// and error() may be accessed immediately.
func NewAddressParser(s string) AddressParser {
	p := AddressParser{s: s}
	i := len(s) - 1
	j := i + 1
	colon := strings.Contains(s, ":")
	for i >= 0 && i < j {
		j = i
		i = p.address(i)
		for i < j && i >= 0 &&
			(s[i] == ',' ||
				(!colon && s[i] == ';')) {
			i--
			i = p.space(i)
		}
	}
	p.Addresses.Uniquify()
	if i < 0 && p.firstError == nil {
		return p
	}

	// Plan B: Look for '@' signs and scan for addresses around
	// them. Use what's there.
	p.Addresses = nil
	leftBorder := 0
	atsign := strings.IndexByte(s, '@')
	for atsign >= 0 {
		nextAtsign := strings.IndexByte(s[atsign+1:], '@')
		rightBorder := 0
		if nextAtsign < 0 {
			rightBorder = len(s)
		} else {
			nextAtsign += atsign + 1
			rightBorder = p.findBorder(atsign+1, nextAtsign-1)
		}
		if leftBorder > 0 &&
			(s[leftBorder] == '.' || s[leftBorder] == '>') {
			leftBorder++
		}
		end := atsign + 1
		for end <= rightBorder && s[end] == ' ' {
			end++
		}
		for end <= rightBorder &&
			((s[end] >= 'a' && s[end] <= 'z') ||
				(s[end] >= 'A' && s[end] <= 'Z') ||
				(s[end] >= '0' && s[end] <= '9') ||
				s[end] == '.' ||
				s[end] == '-') {
			end++
		}
		start := atsign
		for start >= leftBorder && s[start-1] == ' ' {
			start--
		}
		for start <= leftBorder &&
			((s[start-1] >= 'a' && s[start-1] <= 'z') ||
				(s[start-1] >= 'A' && s[start-1] <= 'Z') ||
				(s[start-1] >= '0' && s[start-1] <= '9') ||
				s[start-1] == '.' ||
				s[start-1] == '-') {
			start--
		}
		lp := simplify(s[start:atsign])
		dom := simplify(s[atsign+1 : end])
		if lp != "" && dom != "" {
			addr := NewAddress("", lp, dom)
			p.Addresses = append(p.Addresses, addr)
		}
		atsign = nextAtsign
		leftBorder = rightBorder
	}
	if len(p.Addresses) > 0 {
		p.firstError = nil
		p.recentError = nil
		p.Addresses.Uniquify()
		return p
	}

	// Plan C: Is it an attempt at group syntax by someone who should
	// rather be filling shelves at a supermarket?
	if strings.Contains(s, ":;") && !strings.Contains(s, "@") {
		ix := strings.Index(s, ":;")
		n := simplify(s[:ix])
		j := 0
		bad := false
		var buf bytes.Buffer
		for j < len(n) {
			if (n[j] >= 'a' && n[j] <= 'z') ||
				(n[j] >= 'A' && n[j] <= 'Z') ||
				(n[j] >= '0' && n[j] <= '9') {
				buf.WriteByte(n[j])
			} else if n[j] == ' ' || n[j] == '_' || n[j] == '-' {
				buf.WriteByte('-')
			} else {
				bad = true
			}
			j++
		}
		if !bad {
			p.firstError = nil
			p.recentError = nil
			addr := NewAddress(n, "", "") // FIXME: should this be buf.String() instead of n?
			p.Addresses = []Address{addr}
		}
	}
	return p
}

// Finds the point between \a left and \a right which is most likely to be the
// border between two addresses. Mucho heuristics. Never used for correct
// addresses, only when we're grasping at straws.
//
// Both \a left and \a right are considered to be possible borders, but a
// border between the extremes is preferred if possible.
func (p *AddressParser) findBorder(left, right int) int {
	// if there's only one chance, that _is_ the border.
	if right <= left {
		return left
	}

	// comma?
	b := left + strings.IndexByte(p.s[left:], ',')
	if b >= left && b <= right {
		return b
	}

	// semicolon? perhaps we should also guard against a dot?
	b = left + strings.IndexByte(p.s[left:], ';')
	if b >= left && b <= right {
		return b
	}

	// less-than or greater-than? To: <asdf@asdf.asdf><asdf@asdf.asdf>
	b = left + strings.IndexByte(p.s[left:], '<')
	if b >= left && b <= right {
		return b
	}
	b = left + strings.IndexByte(p.s[left:], '>')
	if b >= left && b <= right {
		return b
	}

	// whitespace?
	b = left
	for b <= right &&
		p.s[b] != ' ' && p.s[b] != '\t' &&
		p.s[b] != '\r' && p.s[b] != '\n' {
		b++
	}
	if b >= left && b <= right {
		return b
	}

	// try to scan for end of the presumed right-hand-side Domain
	b = left
	dot := b
	for b <= right {
		any := false
		for b <= right &&
			((p.s[b] >= 'a' && p.s[b] <= 'z') ||
				(p.s[b] >= 'A' && p.s[b] <= 'Z') ||
				(p.s[b] >= '0' && p.s[b] <= '9') ||
				p.s[b] == '-') {
			any = true
			b++
		}
		// did we see a Domain component at all?
		if !any {
			if b > left && p.s[b-1] == '.' {
				return b - 1 // no, but we just saw a dot, make that the border
			}
			return b // no, and no dot, so put the border here
		}
		if b <= right {
			// if we don't see a dot here, the Domain cannot go on
			if p.s[b] != '.' {
				return b
			}
			dot = b
			b++
			// see if the next Domain component is a top-level Domain
			for _, tld := range tlds {
				l := len(tld)
				if b+l <= right {
					c := p.s[b+l]
					if !(c >= 'a' && c <= 'z') &&
						!(c >= 'A' && c <= 'Z') &&
						!(c >= '0' && c <= '9') {
						if strings.ToLower(p.s[b:l]) == tld {
							return b + l
						}
					}
				}
			}
		}
	}
	// the entire area is legal in a Domain, but we have to draw the
	// line somewhere, so if we've seen one or more dots in the
	// middle, we use the rightmost dot.
	if dot > left && dot < right {
		return dot
	}

	// the entire area is a single word. what can we do?
	if right+1 >= len(p.s) {
		return right
	}
	return left
}

// Asserts that addresses() should return a list of a single regular
// fully-qualified address. error() will return an error message if that isn't
// the case.
func (p *AddressParser) assertSingleAddress() {
	normal := 0
	for _, a := range p.Addresses {
		if a.t == NormalAddressType {
			normal++
			if normal > 1 {
				a.err = fmt.Errorf("This is address no. %d of 1 allowed", normal)
			}
		} else {
			a.err = fmt.Errorf("Expected normal email address (whatever@example.com), got %s", a.toString(false))
		}
	}

	for _, a := range p.Addresses {
		if a.err != nil {
			p.setError(a.err.Error(), 0)
		}
	}

	if len(p.Addresses) == 0 {
		p.setError("No address supplied", 0)
	}
}

// This private helper adds the address with \a name, \a Localpart and \a
// Domain to the list, unless it's there already.
//
// \a name is adjusted heuristically.
func (p *AddressParser) add(name, Localpart, Domain string) {
	// if the Localpart is too long, reject the add()
	if len(Localpart) > 256 {
		p.recentError = fmt.Errorf("Localpart too long (%d characters, RFC 2821's maximum is 64): %s@%s", len(Localpart), Localpart, Domain)
		if p.firstError == nil {
			p.firstError = p.recentError
		}
		return
	}
	// anti-outlook hackery, step 1: remove extra surrounding quotes
	i := 0
	for i < len(name)-1 &&
		(name[i] == name[len(name)-1-i] &&
			(name[i] == '\'' || name[i] == '"')) {
		i++
	}
	if i > 0 {
		name = name[i : len(name)-i]
	}

	// for names, we treat all whitespace equally. "a b" == " a   b "
	name = simplify(name)

	// sometimes a@b (c) is munged as (c) <a@b>, let's unmunge that.
	if len(name) > 1 && name[0] == '(' && name[len(name)-1] == ')' {
		name = simplify(name[1 : len(name)-1])
	}

	// anti-outlook, step 2: if the name is the same as the address,
	// just kill it.
	an := strings.ToTitle(name)
	if an == strings.ToTitle(Localpart) ||
		(len(an) == len(Localpart)+1+len(Domain) &&
			an == strings.ToTitle(Localpart)+"@"+strings.ToTitle(Domain)) {
		name = ""
	}

	a := NewAddress(name, Localpart, Domain)
	a.err = p.recentError
	p.Addresses = append(p.Addresses, a)
}

// This private function parses an address ending at position \a i and adds it
// to the list.
func (p *AddressParser) address(i int) int {
	// we're presumably looking at an address
	p.lastComment = ""
	p.recentError = nil
	i = p.comment(i)
	s := p.s
	for i > 0 && s[i] == ',' {
		i--
		i = p.comment(i)
	}
	for i > 0 && s[i] == '>' && s[i-1] == '>' {
		i--
	}
	if i < 0 {
		// nothing there. error of some sort.
	} else if i > 1 && s[i-1] == '<' && s[i] == '>' {
		// the address is <>. whether that's legal is another matter.
		p.add("", "", "")
		i -= 2
		if i >= 0 && s[i] == '<' {
			i--
		}
		_, i = p.phrase(i)
	} else if i > 2 && s[i] == '>' && s[i-1] == ';' && s[i-2] == ':' {
		// it's a microsoft-broken '<Unknown-Recipient:;>'
		i -= 3
		var name string
		name, i = p.phrase(i)
		p.add(name, "", "")
		if i >= 0 && s[i] == '<' {
			i--
		}
	} else if i > 2 && s[i] == '>' && s[i-1] == ';' &&
		strings.Contains(s[:i], ":@") {
		// it may be a sendmail-broken '<Unknown-Recipient:@x.y;>'
		x := i
		i -= 2
		_, i = p.domain(i)
		if i > 1 && s[i] == '@' && s[i-1] == ':' {
			i -= 2
			var name string
			name, i = p.phrase(i)
			p.add(name, "", "")
			if i >= 0 && s[i] == '<' {
				i--
			}
		} else {
			i = x
		}
	} else if s[i] == '>' {
		// name-addr
		i--
		var dom string
		dom, i = p.domain(i)
		var lp, name string
		if s[i] == '<' {
			lp = dom
			dom = ""
		} else {
			if s[i] == '@' {
				i--
				for i > 0 && s[i] == '@' {
					i--
				}
				aftercomment := i
				i = p.comment(i)
				if i >= 1 && s[i] == ';' {
					j := i - 1
					for j > 0 && p.s[j] == ' ' {
						j--
					}
					if p.s[j] == ':' {
						// <unlisted-recipients:; (no To-header on input)@do.ma.in>
						j--
						n, j := p.phrase(j)
						if n != "" {
							lp = ""
							dom = ""
							name = n
							i = j
						}
					}
				} else if aftercomment > i && i < 0 {
					// To: <(Recipient list suppressed)@localhost>
					n := simplify(p.lastComment)
					lp = ""
					dom = ""
					name = ""
					j := 0
					var buf bytes.Buffer
					for j < len(n) {
						if (n[j] >= 'a' && n[j] <= 'z') ||
							(n[j] >= 'A' && n[j] <= 'Z') ||
							(n[j] >= '0' && n[j] <= '9') {
							buf.WriteByte(n[j])
						} else if n[j] == ' ' || n[j] == '_' || n[j] == '-' {
							buf.WriteByte('-')
						} else {
							p.setError("Localpart contains parenthesis", i)
						}
						j++
					}
					name = buf.String()
				} else {
					lp, i = p.localpart(i)
					if s[i] != '<' {
						j := i
						for j >= 0 &&
							((s[j] >= 'a' && s[j] <= 'z') ||
								(s[j] >= 'A' && s[j] <= 'Z') ||
								s[j] == ' ') {
							j--
						}
						if j >= 0 && s[j] == '<' {
							tmp := s[j+1 : i+1]
							if s[i+1] == ' ' {
								tmp += " "
							}
							lp = tmp + lp
							i = j
						}
					}
				}
			}
			i = p.route(i)
		}
		if i >= 0 && s[i] == '<' {
			i--
			for i >= 0 && s[i] == '<' {
				i--
			}
			var n string
			n, i = p.phrase(i)
			for i >= 0 && (s[i] == '@' || s[i] == '<') {
				// we're looking at an unencoded 8-bit name, or at
				// 'lp@Domain<lp@Domain>', or at 'x<y<z@Domain>'. we
				// react to that by ignoring the display-name.
				i--
				_, i = p.phrase(i)
				n = ""
			}
			if n != "" {
				name = n
			}
		}
		// if the display-name contains unknown-8bit or the
		// undisplayable marker control characters, we drop the
		// display-name.
		j := 0
		for j < len(name) && (name[j] >= 32 && name[j] < 127) {
			j++
		}
		if j < len(name) {
			name = ""
		}
		p.add(name, lp, dom)
	} else if i > 1 && s[i] == '=' && s[i-1] == '?' && s[i-2] == '>' {
		// we're looking at "=?charset?q?safdsafsdfs<a@b>?=". how ugly.
		i -= 3
		var dom string
		dom, i = p.domain(i)
		if s[i] == '@' {
			i--
			for i > 0 && s[i] == '@' {
				i--
			}
			var lp string
			lp, i = p.localpart(i)
			if s[i] == '<' {
				i--
				_, i = p.atom(i) // discard the "supplied" display-name
				p.add("", lp, dom)
			} else {
				p.setError("Expected '<' while in =?...?...<Localpart@Domain>?=", i)
				return i
			}
		} else {
			p.setError("Expected '@' while in =?...?...<Localpart@Domain>?=", i)
			return i
		}
	} else if s[i] == ';' && strings.Contains(s[:i], ":") {
		// group
		empty := true
		i--
		p.comment(i)
		for i > 0 && s[i] != ':' {
			j := i
			i = p.address(i)
			empty = false
			if i == j {
				p.setError("Parsing stopped while in group parser", i)
				return i
			}
			if s[i] == ',' {
				i--
			} else if s[i] != ':' {
				p.setError("Expected : or ',' while parsing group", i)
				return i
			}
		}
		if s[i] == ':' {
			i--
			var name string
			name, i = p.phrase(i)
			if empty {
				p.add(name, "", "")
			}
		}
	} else if s[i] == '"' && strings.Contains(s[:i], "%\"") {
		// quite likely we're looking at x%"y@z", as once used on vms
		x := i
		x--
		dom, x := p.domain(x)
		if x > 0 && s[x] == '@' {
			x--
			lp, x := p.localpart(x)
			if x > 2 && s[x] == '"' && s[x-1] == '%' {
				x -= 2
				_, x := p.domain(x)
				p.add("", lp, dom)
				i = x
			}
		}
	} else if s[i] == '"' && strings.Contains(s[:i], "::") {
		// we may be looking at A::B "display-name"
		b := i - 1
		for b > 0 && s[b] != '"' {
			b--
		}
		name := ""
		if s[b] == '"' {
			var err error
			name, err = decode(s[b+1:i], "us-ascii")
			i = b - 1
			if err != nil { // FIXME: should really check well-formedness instead
				name = ""
			}
			name = "" // do it anyway: we don't want name <Localpart>
		}
		var lp string
		lp, i = p.atom(i)
		if i > 2 && s[i] == ':' && s[i-1] == ':' {
			i -= 2
			var a string
			a, i = p.atom(i)
			lp = a + "::" + lp
			p.add(name, lp, "")
		} else {
			p.setError("Expected NODE::USER while parsing VMS address", i)
		}
	} else if i > 10 && s[i] >= '0' && s[i] <= '9' && s[i-2] == '.' &&
		strings.Contains(s, "\"") && strings.Contains(s, "-19") {
		// we may be looking at A::B "display-name" date
		x := i
		for x > 0 && s[x] != '"' {
			x--
		}
		date := simplify(strings.ToLower(s[x+1 : i]))
		dp := 0
		c := date[0]
		for dp < len(date) &&
			((c >= 'a' && c <= 'z') ||
				(c >= 'A' && c <= 'Z') ||
				(c >= '0' && c <= '9') ||
				c == ' ' || c == '-' ||
				c == ':' || c == '.') {
			dp++
			c = date[dp]
		}
		if dp == len(date) && strings.Contains(date, "-19") {
			// at least it resembles the kind of date field we skip
			i = x
		}
	} else if isQuoted(s, '"', '\'') && strings.Contains(s, "@") {
		wrapped := NewAddressParser(unquote(s, '"', '\''))
		if wrapped.firstError == nil {
			p.Addresses = append(p.Addresses, wrapped.Addresses...)
			i = -1
		} else {
			p.setError("Unexpected quote character", i)
		}
	} else {
		// addr-spec
		name, err := decode(p.lastComment, "us-ascii")
		if err != nil || strings.Contains(p.lastComment, "=?") {
			name = ""
		}
		var dom string
		dom, i = p.domain(i)
		lp := ""
		if s[i] == '@' {
			i--
			for i > 0 && s[i] == '@' {
				i--
			}
			aftercomment := i
			i = p.comment(i)
			if i >= 1 && s[i] == ';' {
				j := i - 1
				for j > 0 && p.s[j] == ' ' {
					j--
				}
				if p.s[j] == ':' {
					// unlisted-recipients:; (no To-header on input)@do.ma.in
					j--
					n, j := p.phrase(j)
					if n != "" {
						lp = ""
						dom = ""
						name = n
						i = j
					}
				}
			} else if aftercomment > i && i < 0 {
				// To: (Recipient list suppressed)@localhost
				n := simplify(p.lastComment)
				lp = ""
				dom = ""
				name = ""
				j := 0
				var buf bytes.Buffer
				for j < len(n) {
					if (n[j] >= 'a' && n[j] <= 'z') ||
						(n[j] >= 'A' && n[j] <= 'Z') ||
						(n[j] >= '0' && n[j] <= '9') {
						buf.WriteByte(n[j])
					} else if n[j] == ' ' || n[j] == '_' || n[j] == '-' {
						buf.WriteByte('-')
					} else {
						p.setError("Localpart contains parenthesis", i)
					}
					j++
				}
			} else {
				lp, i = p.localpart(i)
			}
		} else {
			lp = dom
			dom = ""
		}
		i = p.route(i)
		i = p.comment(i)
		if lp != "" || dom != "" || name != "" {
			p.add(name, lp, dom)
		}
	}
	i = p.comment(i)
	return i
}

// This private function skips past space at position \a i, or past nothing.
// Nothing is perfectly okay.
func (p *AddressParser) space(i int) int {
	for i >= 0 && (p.s[i] == 32 || p.s[i] == 9 ||
		p.s[i] == 13 || p.s[i] == 10) {
		i--
	}
	return i
}

// This private function skips past a sequence of spaces and comments at \a i,
// or past nothing. Nothing is perfectly okay.
func (p *AddressParser) comment(i int) int {
	i = p.space(i)
	for i > 0 && p.s[i] == ')' {
		j := i
		// ctext    = NO-WS-CTL /     ; Non white space controls
		//
		//            %d33-39 /       ; The rest of the US-ASCII
		//            %d42-91 /       ;  characters not including "(",
		//            %d93-126        ;  ")", or "\"
		//
		// ccontent = ctext / quoted-pair / comment
		//
		// comment  = "(" *([FWS] ccontent) [FWS] ")"
		i--
		i = p.ccontent(i)
		if p.s[i] != '(' {
			p.setError("Unbalanced comment: ", i)
		} else {
			ep := NewParser(p.s[i : j+1])
			p.lastComment = ep.Comment()
		}
		if i > 0 {
			i--
			i = p.space(i)
		}
	}
	return i
}

// This very private helper helps comment() handle nested comments. It advances
// \a i to the start of a comment (where it points to '(').
func (p *AddressParser) ccontent(i int) int {
	for {
		if i > 0 && p.s[i-1] == '\\' {
			i--
		} else if p.s[i] == ')' {
			i = p.comment(i)
		} else if p.s[i] == '(' {
			return i
		}

		if i == 0 {
			return i
		}
		i--
	}
}

// This static helper removes quoted-pair from \a s and turns all sequences of
// spaces into a single space. It returns the result.
func unqp(s string) string {
	sp := false
	var buf bytes.Buffer
	j := 0
	for j < len(s) {
		if s[j] == ' ' || s[j] == 9 ||
			s[j] == 10 || s[j] == 13 {
			sp = true
			for s[j] == ' ' || s[j] == 9 ||
				s[j] == 10 || s[j] == 13 {
				j++
			}
		} else {
			if sp {
				buf.WriteByte(' ')
				sp = false
			}
			if s[j] == '\\' {
				j++
				buf.WriteByte(s[j])
				j++
			} else {
				buf.WriteByte(s[j])
				j++
			}
		}
	}
	return buf.String()
}

// This private function picks up a Domain ending at \a i and returns it as a
// string. The validity of the Domain is not checked (and should not be - it
// may come from an old mail message) only its syntactical validity.
func (p *AddressParser) domain(i int) (string, int) {
	i = p.comment(i)

	//Domain         = dot-atom / Domain-literal / obs-Domain
	//Domain-literal = [CFWS] "[" *([FWS] dcontent) [FWS] "]" [CFWS]
	//dcontent       = dtext / quoted-pair
	//dtext          = NO-WS-CTL /     ; Non white space controls
	//                 %d33-90 /       ; The rest of the US-ASCII
	//                 %d94-126        ;  characters not including "[",
	//                                 ;  "]", or "\"

	dom := ""
	if i < 0 {
		return dom, i
	}

	if p.s[i] >= '0' && p.s[i] <= '9' {
		// scan for an unquoted IPv4 address and turn that into an
		// address literal if found.
		j := i
		for (p.s[i] >= '0' && p.s[i] <= '9') || p.s[i] == '.' {
			i--
		}
		test := net.ParseIP(p.s[i+1 : j+1])
		if test != nil {
			return fmt.Sprintf("[%s]", test.String()), i
		}
		i = j
	}

	if p.s[i] == ']' {
		i--
		j := i
		for i >= 0 && p.s[i] != '[' {
			i--
		}
		if i > 0 {
			i--
			// copy the string we fetched, turn FWS into a single
			// space and unquote quoted-pair. we parse forward here
			// because of quoted-pair.
			dom = unqp(p.s[i+1 : j])
		} else {
			p.setError("literal Domain missing [", i)
		}
	} else {
		// atoms, separated by '.' and (obsoletely) spaces. the spaces
		// are stripped.
		dom, i = p.atom(i)
		p.comment(i)
		for i >= 0 && p.s[i] == '.' {
			i--
			var a string
			a, i = p.atom(i)
			if a != "" {
				dom = a + "." + dom
			}
		}
		// FIXME: does this properly handle zero-length Domains?
	}

	return dom, i
}

// This private function parses and returns the atom ending at \a i.
func (p *AddressParser) atom(i int) (string, int) {
	i = p.comment(i)
	j := i
	s := p.s
	for i >= 0 &&
		((s[i] >= 'a' && s[i] <= 'z') ||
			(s[i] >= 'A' && s[i] <= 'Z') ||
			(s[i] >= '0' && s[i] <= '9') ||
			s[i] == '!' || s[i] == '#' ||
			s[i] == '$' || s[i] == '%' ||
			s[i] == '&' || s[i] == '\'' ||
			s[i] == '*' || s[i] == '+' ||
			s[i] == '-' || s[i] == '/' ||
			s[i] == '=' || s[i] == '?' ||
			s[i] == '^' || s[i] == '_' ||
			s[i] == '`' || s[i] == '{' ||
			s[i] == '|' || s[i] == '}' ||
			s[i] == '~' ||
			s[i] >= 128) {
		i--
	}
	r := s[i+1 : j+1]
	i = p.comment(i)
	return r, i
}

// This private function parses an RFC 2822 phrase (a sequence of words, more
// or less) ending at \a i, and returns the phrase as a string.
func (p *AddressParser) phrase(i int) (string, int) {
	r := ""
	i = p.comment(i)
	done := false
	drop := false
	enc := false
	for !done && i >= 0 {
		word := ""
		encw := false
		if i > 0 && p.s[i] == '"' {
			// quoted phrase
			j := i
			i--
			progressing := true
			for progressing {
				if i > 1 && p.s[i-1] == '\\' {
					i -= 2
				} else if i >= 0 && p.s[i] != '"' {
					i--
				} else {
					progressing = false
				}
			}
			if i < 0 || p.s[i] != '"' {
				p.setError("quoted phrase must begin with '\"'", i)
			}
			w := unquote(p.s[i:j+1], '"', '\'')
			l := 0
			for l >= 0 && !drop {
				b := strings.Index(w[l:], "=?")
				if b >= 0 {
					b += l
					e := strings.Index(w[b+2:], "?") // after charset
					if e >= 0 {
						e += b + 2
						e = strings.Index(w[e+1:], "?") // after codec
					}
					if e >= 0 {
						e += e + 1
						e = strings.Index(w[e+1:], "?=") // at the end
					}
					if e >= 0 {
						e += e + 1
						tmp := de2047(w[b : e+2])
						word += w[l:b] + tmp
						if tmp == "" {
							drop = true
						}
						l = e + 2
					} else {
						drop = true
					}
				} else {
					word += w[l:]
					l = -1
				}
			}
			i--
		} else if p.s[i] == '.' {
			// obs-phrase allows a single dot as alternative to word.
			// we allow atom "." as an alternative, too, to handle
			// initials.
			i--
			word, i = p.atom(i)
			word += "."
		} else {
			// single word
			var a string
			a, i = p.atom(i)
			// outlook or something close to it seems to occasionally
			// put backslashes into otherwise unquoted names. work
			// around that:
			l := len(a)
			for l > 0 && i >= 0 && p.s[i] == '\\' {
				i--
				var w string
				w, i = p.atom(i)
				l = len(w)
				a = w + a
			}
			if a == "" {
				done = true
			} else if strings.HasPrefix(a, "=?") {
				p := NewParser(a)
				tmp := simplify(p.Phrase())
				if strings.HasPrefix(tmp, "=?") || strings.Contains(tmp, "=?") {
					drop = true
				}
				if p.AtEnd() {
					word = tmp
					encw = true
				} else {
					word = a
				}
			} else {
				word = a
			}
		}
		if r == "" {
			r = word
		} else if word[len(word)-1] == ' ' {
			r = word + r
		} else if word != "" {
			if !enc || !encw ||
				(len(word)+len(r) < 50 && r[0] <= 'Z') {
				word += " "
			}
			r = word + r
		}
		i = p.comment(i)
		enc = encw
	}
	if drop {
		r = ""
	}
	return simplify(r), i
}

// This private function parses the Localpart ending at \a i, and returns it as
// a string.
func (p *AddressParser) localpart(i int) (string, int) {
	r := ""
	s := ""
	more := true
	if i < 0 {
		more = false
	}
	atomOnly := true
	for more {
		w := ""
		if p.s[i] == '"' {
			atomOnly = false
			w, i = p.phrase(i)
		} else {
			w, i = p.atom(i)
		}
		buf := bytes.NewBufferString(w)
		ds, _ := decode(s, "us-ascii")
		buf.WriteString(ds)
		buf.WriteString(r)
		r = buf.String()
		if i >= 0 && p.s[i] == '.' {
			s = p.s[i : i+1]
			i--
		} else if strings.HasPrefix(w, "%") {
			s = ""
		} else {
			more = false
		}
	}
	if atomOnly && r == "" {
		p.setError("Empty Localpart", i)
	}
	return r, i
}

// If \a i points to an obs-route, this function silently skips the route.
func (p *AddressParser) route(i int) int {
	if i < 0 || p.s[i] != ':' || p.firstError != nil {
		return i
	}

	i--
	var dom string
	dom, i = p.domain(i)
	if dom == "mailto" {
		return i
	}
	for i >= 0 && dom != "" &&
		(p.s[i] == ',' || p.s[i] == '@') {
		if i >= 0 && p.s[i] == '@' {
			i--
		}
		for i >= 0 && p.s[i] == ',' {
			i--
		}
		dom, i = p.domain(i)
	}
	p.firstError = nil
	p.recentError = nil
	return i
}

// This private function records the error \a s, which is considered to occur
// at position \a i.
func (p *AddressParser) setError(s string, i int) {
	if i < 0 {
		i = 0
	}
	start := 0
	if i > 8 {
		start = i - 8
	}
	end := 20
	if end > len(p.s) {
		end = len(p.s)
	}
	nearby := simplify(p.s[start:end])
	p.recentError = fmt.Errorf("%s at position %d (nearby text: %q)", s, i, nearby)
	if p.firstError == nil {
		p.firstError = p.recentError
	}
}

// Removes any addresses that exist twice in the list.
func (as *Addresses) Uniquify() {
	key := func(a Address) string {
		return fmt.Sprintf("%s@%s", strings.ToTitle(a.Localpart), strings.ToTitle(a.Domain))
	}

	if len(*as) == 0 {
		return
	}

	dict := make(map[string]int)
	unique := []Address{}
	for _, a := range *as {
		k := key(a)
		ix, ok := dict[k]
		if !ok {
			dict[k] = len(unique)
			unique = append(unique, a)
		} else if unique[ix].name == "" && a.name != "" {
			unique[ix] = a
		}
	}
	*as = unique
}
