package mailer

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net/mail"
	"strings"
	"time"
)

// buildMessage formats msg as an RFC 5322 message: UTF-8 headers, and a
// quoted-printable text body with an HTML alternative when there is one.
func buildMessage(from, fromName string, msg Message, now time.Time) ([]byte, error) {
	if strings.ContainsAny(msg.Subject, "\r\n") {
		return nil, errors.New("invalid subject")
	}
	var b bytes.Buffer
	header := func(k, v string) { fmt.Fprintf(&b, "%s: %s\r\n", k, v) }
	header("From", (&mail.Address{Name: fromName, Address: from}).String())
	header("To", strings.Join(msg.To, ", "))
	header("Subject", mime.QEncoding.Encode("utf-8", msg.Subject))
	header("Date", now.Format(time.RFC1123Z))
	header("Message-ID", "<"+randomHex(16)+"@"+domainOf(from)+">")
	header("MIME-Version", "1.0")
	// Mail sent by the system, not a person: no auto-replies (RFC 3834).
	header("Auto-Submitted", "auto-generated")

	if msg.HTML == "" {
		header("Content-Type", "text/plain; charset=utf-8")
		header("Content-Transfer-Encoding", "quoted-printable")
		b.WriteString("\r\n")
		writeQP(&b, msg.Text)
		return b.Bytes(), nil
	}
	boundary := "hitstor-" + randomHex(12)
	header("Content-Type", `multipart/alternative; boundary="`+boundary+`"`)
	b.WriteString("\r\n")
	for _, part := range []struct{ typ, body string }{{"text/plain", msg.Text}, {"text/html", msg.HTML}} {
		b.WriteString("--" + boundary + "\r\n")
		header("Content-Type", part.typ+"; charset=utf-8")
		header("Content-Transfer-Encoding", "quoted-printable")
		b.WriteString("\r\n")
		writeQP(&b, part.body)
		b.WriteString("\r\n")
	}
	b.WriteString("--" + boundary + "--\r\n")
	return b.Bytes(), nil
}

func writeQP(b *bytes.Buffer, s string) {
	w := quotedprintable.NewWriter(b)
	w.Write([]byte(strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\n", "\r\n")))
	w.Close()
}

func domainOf(addr string) string {
	if i := strings.LastIndexByte(addr, '@'); i >= 0 {
		return addr[i+1:]
	}
	return "localhost"
}

func randomHex(n int) string {
	buf := make([]byte, n)
	rand.Read(buf)
	return hex.EncodeToString(buf)
}
