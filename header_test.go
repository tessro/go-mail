package mail_test

import (
	"fmt"
	"os"
	"testing"

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
		t.Errorf("incorrect number of Cc addresses: expected 0, got %d", len(to))
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
}
