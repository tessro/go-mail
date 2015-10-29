package mail_test

import (
	"testing"
)

func TestPlainBody(t *testing.T) {
	msg := loadFixture(t, "plain")

	testStringEquals(t, "Text", msg.Text, "This is a simple text email.\r\n")
}

func TestMultipartBody(t *testing.T) {
	msg := loadFixture(t, "multipart")
	/* Structure:
	 * - multipart/related
	 *   - multipart/alternative
	 *	   - text/plain (quoted-printable)
	 *	   - text/html (quoted-printable)
	 *   - image/png (base64)
	 */

	testStringEquals(t, "Message Content-Type", msg.Header.ContentType().Type, "multipart")
	testStringEquals(t, "Message Content-Type subtype", msg.Header.ContentType().Subtype, "related")

	// multipart emails should have empty Text fields
	testStringEquals(t, "Message text", msg.Text, "")

	parts := msg.Parts
	if len(parts) != 2 {
		t.Errorf("incorrect number of message parts: expected 2, got %d", len(parts))
		t.FailNow()
	}

	// first part is multipart/alternative
	testStringEquals(t, "Part 1 Content-Type", parts[0].Header.ContentType().Type, "multipart")
	testStringEquals(t, "Part 1 Content-Type subtype", parts[0].Header.ContentType().Subtype, "alternative")
	testStringEquals(t, "Part 1 text", parts[0].Text, "")

	subparts := parts[0].Parts
	if len(subparts) != 2 {
		t.Errorf("incorrect number of nested parts: expected 2, got %d", len(subparts))
		t.FailNow()
	}

	// first part's first child is text/plain
	testStringEquals(t, "Part 1.1 Content-Type", subparts[0].Header.ContentType().Type, "text")
	testStringEquals(t, "Part 1.1 Content-Type subtype", subparts[0].Header.ContentType().Subtype, "plain")
	testStringEquals(t, "Part 1.1 text", subparts[0].Text, "Cat! 🐱😀\r\n\r\n[image: Inline image 1]\r\n")

	// first part's second child is text/html
	testStringEquals(t, "Part 1.2 Content-Type", subparts[0].Header.ContentType().Type, "text")
	testStringEquals(t, "Part 1.2 Content-Type subtype", subparts[0].Header.ContentType().Subtype, "plain")
	testStringEquals(t, "Part 1.2 text", subparts[1].Text, "<div dir=\"ltr\">Cat!\u00a0🐱😀<div><br></div><div><img src=\"cid:ii_150b178a80ecad03\" alt=\"Inline image 1\" style=\"margin-right: 0px;\"><br clear=\"all\"><div><br></div>\r\n</div></div>\r\n")

	// Image attachment
	testStringEquals(t, "Part 2 Content-Type", parts[1].Header.ContentType().Type, "image")
	testStringEquals(t, "Part 2 Content-Type subtype", parts[1].Header.ContentType().Subtype, "png")
	testStringEquals(t, "Part 2 Content-Type first parameter name", parts[1].Header.ContentType().Parameters[0].Name, "name")
	testStringEquals(t, "Part 2 Content-Type first parameter value", parts[1].Header.ContentType().Parameters[0].Value, "catmustache.png")
	testStringEquals(t, "Part 2 Content-Disposition", parts[1].Header.ContentDisposition().Disposition, "inline")
	testStringEquals(t, "Part 2 Content-Disposition first parameter name", parts[1].Header.ContentDisposition().Parameters[0].Name, "filename")
	testStringEquals(t, "Part 2 Content-Disposition first parameter value", parts[1].Header.ContentDisposition().Parameters[0].Value, "catmustache.png")
	testStringEquals(t, "Part 2 text", parts[1].Text, "")
	// 32756 = byte length of original file
	testIntegerEquals(t, "Part 2 data size", len(parts[1].Data), 32756)
}

func TestMalformedMultipartTerminator(t *testing.T) {
	msg := loadFixture(t, "bad-multipart")
	/* Structure:
	 * - multipart/related
	 *   - multipart/alternative
	 *	   - text/plain (quoted-printable)
	 *	   - text/html (quoted-printable)
	 *   - image/png (base64)
	 */

	testStringEquals(t, "Message Content-Type", msg.Header.ContentType().Type, "multipart")
	testStringEquals(t, "Message Content-Type subtype", msg.Header.ContentType().Subtype, "related")

	// multipart emails should have empty Text fields
	testStringEquals(t, "Message text", msg.Text, "")

	parts := msg.Parts
	if len(parts) != 2 {
		t.Errorf("incorrect number of message parts: expected 2, got %d", len(parts))
		t.FailNow()
	}

	// first part is multipart/alternative
	testStringEquals(t, "Part 1 Content-Type", parts[0].Header.ContentType().Type, "multipart")
	testStringEquals(t, "Part 1 Content-Type subtype", parts[0].Header.ContentType().Subtype, "alternative")
	testStringEquals(t, "Part 1 text", parts[0].Text, "")

	subparts := parts[0].Parts
	if len(subparts) != 2 {
		t.Errorf("incorrect number of nested parts: expected 2, got %d", len(subparts))
		t.FailNow()
	}

	// first part's first child is text/plain
	testStringEquals(t, "Part 1.1 Content-Type", subparts[0].Header.ContentType().Type, "text")
	testStringEquals(t, "Part 1.1 Content-Type subtype", subparts[0].Header.ContentType().Subtype, "plain")
	testStringEquals(t, "Part 1.1 text", subparts[0].Text, "Cat! 🐱😀\r\n\r\n[image: Inline image 1]\r\n")

	// first part's second child is text/html
	testStringEquals(t, "Part 1.2 Content-Type", subparts[0].Header.ContentType().Type, "text")
	testStringEquals(t, "Part 1.2 Content-Type subtype", subparts[0].Header.ContentType().Subtype, "plain")
	testStringEquals(t, "Part 1.2 text", subparts[1].Text, "<div dir=\"ltr\">Cat!\u00a0🐱😀<div><br></div><div><img src=\"cid:ii_150b178a80ecad03\" alt=\"Inline image 1\" style=\"margin-right: 0px;\"><br clear=\"all\"><div><br></div>\r\n</div></div>\r\n")

	// Image attachment
	testStringEquals(t, "Part 2 Content-Type", parts[1].Header.ContentType().Type, "image")
	testStringEquals(t, "Part 2 Content-Type subtype", parts[1].Header.ContentType().Subtype, "png")
	testStringEquals(t, "Part 2 Content-Type first parameter name", parts[1].Header.ContentType().Parameters[0].Name, "name")
	testStringEquals(t, "Part 2 Content-Type first parameter value", parts[1].Header.ContentType().Parameters[0].Value, "catmustache.png")
	testStringEquals(t, "Part 2 Content-Disposition", parts[1].Header.ContentDisposition().Disposition, "inline")
	testStringEquals(t, "Part 2 Content-Disposition first parameter name", parts[1].Header.ContentDisposition().Parameters[0].Name, "filename")
	testStringEquals(t, "Part 2 Content-Disposition first parameter value", parts[1].Header.ContentDisposition().Parameters[0].Value, "catmustache.png")
	testStringEquals(t, "Part 2 text", parts[1].Text, "")
	// 32756 = byte length of original file
	testIntegerEquals(t, "Part 2 data size", len(parts[1].Data), 32756)
}
