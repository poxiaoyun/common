package email

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/smtp"
	"net/url"
	"strings"
	"time"
)

type EmailAddress struct {
	Name  string
	Email string
}

// String returns the email address in the format "Name <email>"
// RFC 5322 format
// see: https://datatracker.ietf.org/doc/html/rfc5322#section-3.4
func (e *EmailAddress) String() string {
	if e.Name == "" {
		return e.Email
	}
	return fmt.Sprintf("%s <%s>", e.Name, e.Email)
}

type EmailAddressList []EmailAddress

func (e EmailAddressList) String() string {
	list := make([]string, len(e))
	for i, v := range e {
		list[i] = v.String()
	}
	return strings.Join(list, ", ")
}

type Email struct {
	// header
	From    EmailAddress
	To      EmailAddressList
	Cc      EmailAddressList
	Date    time.Time
	Subject string
	// body
	ContentType string
	Body        io.Reader
}

func (e *Email) Validate() error {
	if e.From.Email == "" {
		return fmt.Errorf("from is required")
	}
	if len(e.To) == 0 {
		return fmt.Errorf("to is required")
	}
	if e.Subject == "" {
		return fmt.Errorf("subject is required")
	}
	if e.Body == nil {
		return fmt.Errorf("body is required")
	}
	if e.Date.IsZero() {
		e.Date = time.Now()
	}
	return nil
}

func (e *Email) Message() []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "From: %s\r\n", e.From.String())
	fmt.Fprintf(&b, "To: %s\r\n", e.To.String())
	if len(e.Cc) > 0 {
		fmt.Fprintf(&b, "Cc: %s\r\n", e.Cc.String())
	}
	fmt.Fprintf(&b, "Date: %s\r\n", e.Date.Format(time.RFC1123Z))
	fmt.Fprintf(&b, "Subject: %s\r\n", e.Subject)
	if e.ContentType != "" {
		fmt.Fprintf(&b, "Content-Type: %s\r\n", e.ContentType)
	}
	fmt.Fprintf(&b, "\r\n")
	io.Copy(&b, e.Body)
	return b.Bytes()
}

type SMTPOptions struct {
	Address  string `json:"address,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	Identity string `json:"identity,omitempty"`
	Insecure bool   `json:"insecure,omitempty"`
}

// SendMail sends an email using the provided smtp options and email
// if email is nil, it will only connect to the smtp server and verify the connection
func SendMail(ctx context.Context, smtpOptions SMTPOptions, email *Email) error {
	if smtpOptions.Address == "" {
		return fmt.Errorf("address is required")
	}
	u, err := url.Parse(smtpOptions.Address)
	if err != nil {
		return fmt.Errorf("invalid smtp address: %v", err)
	}
	cli, err := smtp.Dial(u.Host)
	if err != nil {
		return err
	}
	if u.Scheme == "smtps" || u.Scheme == "" {
		if ok, _ := cli.Extension("STARTTLS"); !ok {
			return fmt.Errorf("server does not support tls, but tls is required")
		}
		tlsConfig := &tls.Config{}
		if smtpOptions.Insecure {
			tlsConfig.InsecureSkipVerify = true
		}
		if err := cli.StartTLS(tlsConfig); err != nil {
			return err
		}
	}
	if smtpOptions.Username != "" && smtpOptions.Password != "" {
		auth := smtp.PlainAuth(smtpOptions.Identity, smtpOptions.Username, smtpOptions.Password, u.Host)
		if ok, _ := cli.Extension("AUTH"); !ok {
			return fmt.Errorf("server does not support auth, but username and password are provided")
		}
		if err := cli.Auth(auth); err != nil {
			return err
		}
	}
	if email != nil {
		if err := email.Validate(); err != nil {
			return err
		}
		if err := cli.Mail(email.From.Email); err != nil {
			return err
		}
		for _, v := range email.To {
			if err := cli.Rcpt(v.Email); err != nil {
				return err
			}
		}
		for _, v := range email.Cc {
			if err := cli.Rcpt(v.Email); err != nil {
				return err
			}
		}
		w, err := cli.Data()
		if err != nil {
			return err
		}
		defer w.Close()
		_, err = w.Write([]byte(email.Message()))
		if err != nil {
			return err
		}
	}
	return nil
}
