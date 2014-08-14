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

func TestHeader(t *testing.T) {
	msg := loadFixture(t, "basic")

	if msg.Header == nil {
		t.Fatal("missing Header struct")
	}

	ct := msg.Header.ContentType()
	if ct == nil {
		t.Error("missing Content-Type")
	} else if ct.Type != "text" || ct.Subtype != "plain" {
		t.Errorf("incorrect Content-Type: expected text/plain, got %s/%s", ct.Type, ct.Subtype)
	}

	from := msg.Header.Addresses("From")
	if len(from) != 3 {
		t.Errorf("incorrect number of From addresses: expected 2, got %d", len(from))
	} else if from[0].String() != "basic.from@example.com" {
		t.Errorf("incorrect From address: expected basic.from@example.com, got %s", from[1].String())
	} else if from[1].String() != `Full From <full.from@example.com>` {
		t.Errorf("incorrect From address: expected \"Full From <full.from@example.com>\", got %s", from[1].String())
	} else if from[2].String() != "broken.from@example.com" {
		t.Errorf("incorrect From address: expected broken.from@example.com, got %s", from[2].String())
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
