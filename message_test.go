package mail_test

import (
	"testing"
)

func TestPlainBody(t *testing.T) {
	msg := loadFixture(t, "plain")

	testStringEquals(t, "Text", msg.Text, "This is a simple text email.\r\n")
}
