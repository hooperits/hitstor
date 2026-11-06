package mailer

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
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
	for _, in := range msg.Inline {
		if strings.ContainsAny(in.CID+in.ContentType+in.Name, "\r\n\"<>") || in.CID == "" {
			return nil, errors.New("invalid inline image")
		}
	}
	for _, a := range msg.Attachments {
		if strings.ContainsAny(a.Name+a.ContentType, "\r\n\"<>\\") || a.Name == "" {
			return nil, errors.New("invalid attachment")
		}
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
	if len(msg.Attachments) == 0 {
		writeBody(&b, msg)
		return b.Bytes(), nil
	}
	// multipart/mixed: the body, then the attachments.
	mixed := "hitstor-" + randomHex(12)
	header("Content-Type", `multipart/mixed; boundary="`+mixed+`"`)
	b.WriteString("\r\n--" + mixed + "\r\n")
	writeBody(&b, msg)
	for _, a := range msg.Attachments {
		b.WriteString("\r\n--" + mixed + "\r\n")
		header("Content-Type", a.ContentType+`; name="`+a.Name+`"`)
		header("Content-Transfer-Encoding", "base64")
		header("Content-Disposition", `attachment; filename="`+a.Name+`"`)
		b.WriteString("\r\n")
		writeBase64(&b, a.Data)
	}
	b.WriteString("--" + mixed + "--\r\n")
	return b.Bytes(), nil
}

// writeBody writes the message's body, from its Content-Type header: the
// text alone, or multipart/alternative with the text and the HTML (with
// its images in a multipart/related).
func writeBody(b *bytes.Buffer, msg Message) {
	header := func(k, v string) { fmt.Fprintf(b, "%s: %s\r\n", k, v) }
	if msg.HTML == "" {
		header("Content-Type", "text/plain; charset=utf-8")
		header("Content-Transfer-Encoding", "quoted-printable")
		b.WriteString("\r\n")
		writeQP(b, msg.Text)
		return
	}
	boundary := "hitstor-" + randomHex(12)
	header("Content-Type", `multipart/alternative; boundary="`+boundary+`"`)
	b.WriteString("\r\n--" + boundary + "\r\n")
	header("Content-Type", "text/plain; charset=utf-8")
	header("Content-Transfer-Encoding", "quoted-printable")
	b.WriteString("\r\n")
	writeQP(b, msg.Text)
	b.WriteString("\r\n--" + boundary + "\r\n")
	related := ""
	if len(msg.Inline) > 0 {
		related = "hitstor-" + randomHex(12)
		header("Content-Type", `multipart/related; type="text/html"; boundary="`+related+`"`)
		b.WriteString("\r\n--" + related + "\r\n")
	}
	header("Content-Type", "text/html; charset=utf-8")
	header("Content-Transfer-Encoding", "quoted-printable")
	b.WriteString("\r\n")
	writeQP(b, msg.HTML)
	b.WriteString("\r\n")
	for _, in := range msg.Inline {
		b.WriteString("--" + related + "\r\n")
		header("Content-Type", in.ContentType)
		header("Content-Transfer-Encoding", "base64")
		header("Content-ID", "<"+in.CID+">")
		header("Content-Disposition", `inline; filename="`+in.Name+`"`)
		b.WriteString("\r\n")
		writeBase64(b, in.Data)
	}
	if related != "" {
		b.WriteString("--" + related + "--\r\n")
	}
	b.WriteString("--" + boundary + "--\r\n")
}

// writeBase64 writes data in base64 lines of 76 characters (RFC 2045).
func writeBase64(b *bytes.Buffer, data []byte) {
	enc := base64.StdEncoding.EncodeToString(data)
	for len(enc) > 76 {
		b.WriteString(enc[:76] + "\r\n")
		enc = enc[76:]
	}
	b.WriteString(enc + "\r\n")
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
