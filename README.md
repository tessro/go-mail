# go-mail

**Important note: go-mail is EXPERIMENTAL.** It parses many messages correctly, but may return incorrect results or even crash with certain inputs. Many code paths lack test coverage. The API surface may change. You’ve been warned! :)

[![Build Status](https://travis-ci.org/paulrosania/go-mail.svg?branch=master)](https://travis-ci.org/paulrosania/go-mail)

![xkcd 1349: Shouldn’t Be Hard](http://imgs.xkcd.com/comics/shouldnt_be_hard.png)

go-mail is a robust RFC5322/2822/822 message parser.

## Motivation

Email parsing is a pain. It spans many RFCs (e.g. [5322](https://tools.ietf.org/html/rfc5322), [2045](https://tools.ietf.org/html/rfc2045), [2046](https://tools.ietf.org/html/rfc2046), [2047](https://tools.ietf.org/html/rfc2047), [4289](https://tools.ietf.org/html/rfc4289), [2049](https://tools.ietf.org/html/rfc2049), [4288](https://tools.ietf.org/html/rfc4288), [4021](https://tools.ietf.org/html/rfc4021), [6532](https://tools.ietf.org/html/rfc6532)) and includes joys like:

* Comment-folding white space
* 7-bit encoding
* Content transfer encodings
	* quoted printable
	* base64
* Encoded words (`=?utf-8?q?email=20is=20hard?=`)
* Multipart messages

… and that’s when things going smoothly. In practice, malformed messages are commonplace. go-mail accepts this reality and makes a best effort to interpret each message based on the sender’s intent.

## Installation

    go get github.com/paulrosania/go-mail

## Documentation

Full API documentation is available here:

[https://godoc.org/github.com/paulrosania/go-mail](https://godoc.org/github.com/paulrosania/go-mail)

## Contributing

1. Fork the project
2. Make your changes
2. Run tests (`go test`)
3. Send a pull request!

If you’re making a big change, please open an issue first, so we can discuss.

## Thanks

* [Archiveopteryx](http://archiveopteryx.org/), for providing a liberally-licensed, robust C++ parser. Much of the original go-mail implementation was derived from it.
* Roger Peppe, for the original [go-charset](https://code.google.com/p/go-charset) library. (go-mail uses a [fork](https://github.com/paulrosania/go-charset).)

## License

go-mail is provided under the MIT license. Major portions of the original implementation were derived from the excellent [Archiveopteryx](http://archiveopteryx.org/) RFC2822 parser, which is provided under the PostgreSQL license. See the [LICENSE](LICENSE) file for details.