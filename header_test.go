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
	} else if from[0].String() != "basic.from@example.com" {
		t.Errorf("incorrect From address: expected basic.from@example.com, got %s", from[1].String())
	} else if from[1].String() != `Full From <full.from@example.com>` {
		t.Errorf("incorrect From address: expected \"Full From <full.from@example.com>\", got %s", from[1].String())
	} else if from[2].String() != "broken.from@example.com" {
		t.Errorf("incorrect From address: expected broken.from@example.com, got %s", from[2].String())
	} else if from[3].String() != "second.broken@example.com" {
		t.Errorf("incorrect From address: expected second.broken@example.com, got %s", from[3].String())
	} else if from[4].String() != "third.broken@example.com" {
		t.Errorf("incorrect From address: expected third.broken@example.com, got %s", from[4].String())
	}

	to := msg.Header.Addresses("To")
	if len(to) != 1 {
		t.Errorf("incorrect number of To addresses: expected 1, got %d", len(to))
	} else if to[0].String() != "recipient@example.com" {
		t.Errorf("incorrect To address: expected recipient@example.com, got %s", to[0].String())
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
}
