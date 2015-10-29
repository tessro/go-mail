package mail_test

import (
	"fmt"
	"os"
	"testing"
	"time"

	"io/ioutil"

	"github.com/paulrosania/go-mail"
)

func loadFixture(t *testing.T, name string) *mail.Message {
	filename := fmt.Sprintf("fixtures/%s.eml", name)
	f, err := os.Open(filename)
	if err != nil {
		t.Fatal(err)
	}

	body, err := ioutil.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}

	msg, err := mail.ReadMessage(string(body))
	if err != nil {
		t.Fatal(err)
	}

	if msg == nil {
		t.Fatalf("**VERY BAD** ReadMessage returned nil with no error")
	}

	return msg
}

func testStringEquals(t *testing.T, field, actual, expected string) {
	if actual != expected {
		t.Errorf("incorrect %s:\nexpected %q,\n     got %q", field, expected, actual)
	}
}

func testIntegerEquals(t *testing.T, field string, actual, expected int) {
	if actual != expected {
		t.Errorf("incorrect %s:\nexpected %d,\n     got %d", field, expected, actual)
	}
}

// Error(args ...interface{})
// Errorf(format string, args ...interface{})
// Fail()
// FailNow()
// Failed() bool
// Fatal(args ...interface{})
// Fatalf(format string, args ...interface{})
// Log(args ...interface{})
// Logf(format string, args ...interface{})
// Skip(args ...interface{})
// SkipNow()
// Skipf(format string, args ...interface{})
// Skipped() bool

func TestContentType(t *testing.T) {
	msg := loadFixture(t, "basic")

	if msg.Header == nil {
		t.Fatal("missing Header struct")
	}

	ct := msg.Header.ContentType()
	if ct == nil {
		t.Error("missing Content-Type")
	} else if ct.Type != "text" || ct.Subtype != "html" {
		t.Errorf("incorrect Content-Type: expected text/html, got %s/%s", ct.Type, ct.Subtype)
	}
}

func TestAddressFields(t *testing.T) {
	msg := loadFixture(t, "basic")

	if msg.Header == nil {
		t.Fatal("missing Header struct")
	}

	from := msg.Header.Addresses("From")
	if len(from) != 5 {
		t.Errorf("incorrect number of From addresses: expected 5, got %d", len(from))
	} else {
		testStringEquals(t, "From address", from[0].String(), "basic.from@example.com")
		testStringEquals(t, "From address", from[1].String(), "Full From <full.from@example.com>")
		testStringEquals(t, "From address", from[2].String(), "broken.from@example.com")
		testStringEquals(t, "From address", from[3].String(), "second.broken@example.com")
		testStringEquals(t, "From address", from[4].String(), "third.broken@example.com")
	}

	to := msg.Header.Addresses("To")
	if len(to) != 1 {
		t.Errorf("incorrect number of To addresses: expected 1, got %d", len(to))
	} else {
		testStringEquals(t, "To address", to[0].String(), "recipient@example.com")
	}

	// Test requests for missing address headers
	cc := msg.Header.Addresses("Cc")
	if len(cc) != 0 {
		t.Errorf("incorrect number of Cc addresses: expected 0, got %d", len(cc))
	}
}

func TestCFWS(t *testing.T) {
	msg := loadFixture(t, "cfws")

	if msg.Header == nil {
		t.Fatal("missing Header struct")
	}

	from := msg.Header.Addresses("From")
	if len(from) != 1 {
		t.Errorf("incorrect number of From addresses: expected 1, got %d", len(from))
	} else if from[0].String() != "Pete <pete@silly.test>" {
		t.Errorf("incorrect From address: expected \"Pete <pete@silly.test>\", got %s", from[0].String())
	}

	to := msg.Header.Addresses("To")
	if len(to) != 3 {
		t.Errorf("incorrect number of To addresses: expected 3, got %d", len(to))
	} else {
		testStringEquals(t, "To address", to[0].String(), "Chris Jones <c@public.example>")
		testStringEquals(t, "To address", to[1].String(), "joe@example.org")
		testStringEquals(t, "To address", to[2].String(), "John <jdoe@one.test>")
	}

	cc := msg.Header.Addresses("Cc")
	if len(cc) != 0 {
		t.Errorf("incorrect number of Cc addresses: expected 0, got %d", len(cc))
	}

	date := msg.Header.Date()
	if date == nil {
		t.Errorf("missing or invalid date field in header")
	} else {
		testStringEquals(t, "Date", date.Format(time.RFC822), "13 Feb 69 23:32 -0330")
	}

	messageID := msg.Header.MessageID()
	testStringEquals(t, "Message-ID", messageID, "<testabcd.1234@silly.test>")
}

// Relevant RFC: https://tools.ietf.org/html/rfc2047
func TestEncodedWords(t *testing.T) {
	msg := loadFixture(t, "encoded-words")

	if msg.Header == nil {
		t.Fatal("missing Header struct")
	}

	subject := msg.Header.Subject()
	if subject != "Testing encoded words! ☺" {
		t.Errorf("incorrect Subject: expected \"Testing encoded words! ☺\", got %s", subject)
	}

	from := msg.Header.Addresses("From")
	if len(from) != 1 {
		t.Errorf("incorrect number of From addresses: expected 1, got %d", len(from))
	} else if from[0].String() != "invalid quotes <invalid.quotes@example.com>" {
		t.Errorf("incorrect From address: expected \"invalid quotes <invalid.quotes@example.com>\", got %s", from[0].String())
	}

	// Test for non-standard "Q"-encoding: the Reply-To header in this fixture
	// contains a '.' in a Q-encoded word within a phrase.
	//
	// From RFC 2047:
	//
	//   An 'encoded-word' may appear in a message header or body part header
	//   according to the following rules:
	//
	//   [...]
	//
	//   (3) As a replacement for a 'word' entity within a 'phrase', for example,
	//   one that precedes an address in a From, To, or Cc header.  The ABNF
	//   definition for 'phrase' from RFC 822 thus becomes:
	//
	//   phrase = 1*( encoded-word / word )
	//
	//   In this case the set of characters that may be used in a "Q"-encoded
	//   'encoded-word' is restricted to: <upper and lower case ASCII
	//   letters, decimal digits, "!", "*", "+", "-", "/", "=", and "_"
	//   (underscore, ASCII 95.)>.  An 'encoded-word' that appears within a
	//   'phrase' MUST be separated from any adjacent 'word', 'text' or
	//   'special' by 'linear-white-space'.
	//
	// Note: go-mail outputs a quoted name, since dots in phrases are obsolete.
	//       (See RFC 5322 Section 4.1)
	replyTo := msg.Header.Addresses("Reply-To")
	if len(replyTo) != 1 {
		t.Errorf("incorrect number of Reply-To addresses: expected 1, got %d", len(replyTo))
	} else if replyTo[0].String() != `"contains an in.valid dot" <invalid.dot@example.com>` {
		t.Errorf("incorrect Reply-To address: expected \"\"contains an in.valid dot\" <invalid.dot@example.com>\", got %s", replyTo[0].String())
	}

	// Test valid "Q"-encoding
	to := msg.Header.Addresses("To")
	if len(to) != 2 {
		t.Errorf("incorrect number of To addresses: expected 2, got %d", len(to))
	} else {
		if to[0].String() != "valid <valid@example.com>" {
			t.Errorf("incorrect To address: expected \"valid <valid@example.com>\", got %s", to[0].String())
		}
		if to[1].String() != "mixed example <mixed@example.com>" {
			t.Errorf("incorrect To address: expected \"mixed example <mixed@example.com>\", got %s", to[1].String())
		}
	}
}

func TestMessageID(t *testing.T) {
	msg := loadFixture(t, "message-id")

	if msg.Header == nil {
		t.Fatal("missing Header struct")
	}

	msgid := msg.Header.MessageID()
	if msgid != "<valid@message-id>" {
		t.Errorf("incorrect Message-ID: expected <valid@message-id>, got %s", msgid)
	}

	parts := msg.Parts
	if len(parts) != 2 {
		t.Errorf("incorrect number of message parts: expected 2, got %d", len(parts))
		t.FailNow()
	}

	testStringEquals(t, "Part 1 Content-ID", parts[0].Header.Fields[2].Name(), "Content-ID")
	testStringEquals(t, "Part 1 Content-ID", parts[0].Header.Fields[2].Value(), "<invalid-id-with-no-brackets>")

	testStringEquals(t, "Part 2 Content-ID", parts[1].Header.Fields[2].Name(), "Content-ID")
	testStringEquals(t, "Part 2 Content-ID", parts[1].Header.Fields[2].Value(), "<valid-id@example>")
}
