package endpoint

import (
	"bytes"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"time"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/forms"
)

// mailer sends submissions over SMTP. It always uses TLS unless the server is
// on this machine and the config says tls: none, and it never lets anything a
// visitor typed into a header: the subject has already been made one line,
// and Reply-To is an address that passed validation.
type mailer struct {
	cfg  SMTPConfig
	send func(addr string, from string, to []string, msg []byte) error // replaced in tests
}

func newMailer(c SMTPConfig) *mailer {
	m := &mailer{cfg: c}
	m.send = m.smtpSend
	return m
}

func (m *mailer) deliver(siteName string, s *forms.Submission) error {
	from, err := mail.ParseAddress(m.cfg.From)
	if err != nil {
		return fmt.Errorf("smtp from %q is not an address", m.cfg.From)
	}
	to, err := mail.ParseAddress(s.Form.To)
	if err != nil {
		return err
	}
	var b bytes.Buffer
	hdr := func(k, v string) { fmt.Fprintf(&b, "%s: %s\r\n", k, v) }
	hdr("From", from.String())
	hdr("To", to.String())
	if s.Form.ReplyTo != "" {
		if a := s.Values[s.Form.ReplyTo]; a != "" {
			hdr("Reply-To", (&mail.Address{Address: a}).String())
		}
	}
	hdr("Subject", mime.QEncoding.Encode("utf-8", s.Subject()))
	hdr("Date", time.Now().Format(time.RFC1123Z))
	id := make([]byte, 12)
	_, _ = rand.Read(id)
	domain := from.Address[strings.LastIndex(from.Address, "@")+1:]
	hdr("Message-ID", "<"+hex.EncodeToString(id)+"@"+domain+">")
	hdr("MIME-Version", "1.0")
	hdr("Content-Type", "text/plain; charset=utf-8")
	hdr("Content-Transfer-Encoding", "quoted-printable")
	hdr("Auto-Submitted", "auto-generated")
	b.WriteString("\r\n")
	qp := quotedprintable.NewWriter(&b)
	body := s.Text() + "\n--\nSent from the " + s.Form.Name + " form on " + siteName + ".\n"
	_, _ = qp.Write([]byte(strings.ReplaceAll(body, "\n", "\r\n")))
	_ = qp.Close()
	addr := net.JoinHostPort(m.cfg.Host, strconv.Itoa(m.port()))
	return m.send(addr, from.Address, []string{to.Address}, b.Bytes())
}

func (m *mailer) port() int {
	if m.cfg.Port != 0 {
		return m.cfg.Port
	}
	if m.cfg.TLS == "tls" {
		return 465
	}
	return 587
}

func (m *mailer) smtpSend(addr, from string, to []string, msg []byte) error {
	tc := &tls.Config{ServerName: m.cfg.Host, MinVersion: tls.VersionTLS12}
	var c *smtp.Client
	if m.cfg.TLS == "tls" {
		conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 15 * time.Second}, "tcp", addr, tc)
		if err != nil {
			return err
		}
		if c, err = smtp.NewClient(conn, m.cfg.Host); err != nil {
			return err
		}
	} else {
		conn, err := net.DialTimeout("tcp", addr, 15*time.Second)
		if err != nil {
			return err
		}
		if c, err = smtp.NewClient(conn, m.cfg.Host); err != nil {
			return err
		}
		if m.cfg.TLS == "starttls" {
			if err := c.StartTLS(tc); err != nil {
				c.Close()
				return fmt.Errorf("starttls: %w", err)
			}
		}
	}
	defer c.Close()
	if m.cfg.Username != "" {
		// PlainAuth refuses to send the password over a connection that
		// isn't encrypted, unless the server is on this machine.
		if err := c.Auth(smtp.PlainAuth("", m.cfg.Username, os.Getenv(m.cfg.PasswordEnv), m.cfg.Host)); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}
	if err := c.Mail(from); err != nil {
		return err
	}
	for _, r := range to {
		if err := c.Rcpt(r); err != nil {
			return err
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}
