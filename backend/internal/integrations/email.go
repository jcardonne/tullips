package integrations

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/emersion/go-imap"
	imapclient "github.com/emersion/go-imap/client"
	"golang.org/x/net/html"
	"golang.org/x/oauth2"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"
)

type Mailbox struct {
	Email       string `json:"email"`
	Username    string `json:"username"`
	Password    string `json:"password,omitempty"`
	AccessToken string `json:"accessToken,omitempty"`
	SMTPHost    string `json:"smtpHost"`
	SMTPPort    int    `json:"smtpPort"`
	IMAPHost    string `json:"imapHost"`
	IMAPPort    int    `json:"imapPort"`
}
type Email struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Subject   string `json:"subject"`
	Text      string `json:"text"`
	MessageID string `json:"messageId"`
	InReplyTo string `json:"inReplyTo,omitempty"`
}

func validHeader(s string) bool { return !strings.ContainsAny(s, "\r\n\x00") }
func FormatEmail(m Email) ([]byte, error) {
	for _, s := range []string{m.From, m.To, m.Subject, m.MessageID, m.InReplyTo} {
		if !validHeader(s) {
			return nil, errors.New("invalid email header")
		}
	}
	from, e := mail.ParseAddress(m.From)
	if e != nil {
		return nil, e
	}
	to, e := mail.ParseAddress(m.To)
	if e != nil {
		return nil, e
	}
	if m.MessageID == "" || strings.ContainsAny(m.MessageID, "<> \t") {
		return nil, errors.New("stable message ID required")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\nTo: %s\r\nSubject: %s\r\nMessage-ID: <%s>\r\nDate: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: quoted-printable\r\n", from.String(), to.String(), mime.QEncoding.Encode("utf-8", m.Subject), m.MessageID, time.Now().Format(time.RFC1123Z))
	if m.InReplyTo != "" {
		fmt.Fprintf(&b, "In-Reply-To: %s\r\nReferences: %s\r\n", m.InReplyTo, m.InReplyTo)
	}
	b.WriteString("\r\n")
	w := quotedprintable.NewWriter(&b)
	_, e = w.Write([]byte(m.Text))
	if e != nil {
		return nil, e
	}
	if e = w.Close(); e != nil {
		return nil, e
	}
	return []byte(b.String()), nil
}
func dialMail(ctx context.Context, host string, port int) (net.Conn, error) {
	if port != 465 && port != 587 && port != 993 {
		return nil, errors.New("mail port must be 465, 587 or 993")
	}
	ips, e := net.DefaultResolver.LookupIPAddr(ctx, host)
	if e != nil {
		return nil, e
	}
	if len(ips) == 0 {
		return nil, errors.New("mail server has no address")
	}
	for _, ip := range ips {
		if !PublicIP(ip.IP) {
			return nil, errors.New("private mail server rejected")
		}
	}
	c, e := (&net.Dialer{Timeout: 15 * time.Second}).DialContext(ctx, "tcp", net.JoinHostPort(ips[0].IP.String(), fmt.Sprint(port)))
	if e == nil {
		deadline := time.Now().Add(45 * time.Second)
		if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
			deadline = d
		}
		c.SetDeadline(deadline)
	}
	return c, e
}

type smtpOAuth struct{ user, token string }

func (a smtpOAuth) Start(s *smtp.ServerInfo) (string, []byte, error) {
	if !s.TLS {
		return "", nil, errors.New("OAuth requires TLS")
	}
	return "XOAUTH2", []byte("user=" + a.user + "\x01auth=Bearer " + a.token + "\x01\x01"), nil
}
func (a smtpOAuth) Next(_ []byte, more bool) ([]byte, error) {
	if more {
		return nil, errors.New("OAuth authentication rejected")
	}
	return nil, nil
}

type imapOAuth struct{ user, token string }

func (a imapOAuth) Start() (string, []byte, error) {
	return "XOAUTH2", []byte("user=" + a.user + "\x01auth=Bearer " + a.token + "\x01\x01"), nil
}
func (a imapOAuth) Next(_ []byte) ([]byte, error) {
	return nil, errors.New("OAuth authentication rejected")
}
func (m Mailbox) smtp(ctx context.Context) (*smtp.Client, error) {
	if m.SMTPPort != 465 && m.SMTPPort != 587 {
		return nil, errors.New("SMTP requires port 465 or 587")
	}
	conn, e := dialMail(ctx, m.SMTPHost, m.SMTPPort)
	if e != nil {
		return nil, e
	}
	cfg := &tls.Config{ServerName: m.SMTPHost, MinVersion: tls.VersionTLS12}
	if m.SMTPPort == 465 {
		tc := tls.Client(conn, cfg)
		if e = tc.HandshakeContext(ctx); e != nil {
			conn.Close()
			return nil, e
		}
		conn = tc
	}
	c, e := smtp.NewClient(conn, m.SMTPHost)
	if e != nil {
		conn.Close()
		return nil, e
	}
	if m.SMTPPort == 587 {
		if e = c.StartTLS(cfg); e != nil {
			c.Close()
			return nil, e
		}
	}
	user := m.Username
	if user == "" {
		user = m.Email
	}
	var auth smtp.Auth = smtp.PlainAuth("", user, m.Password, m.SMTPHost)
	if m.AccessToken != "" {
		auth = smtpOAuth{user, m.AccessToken}
	}
	if e = c.Auth(auth); e != nil {
		c.Close()
		return nil, e
	}
	return c, nil
}
func (m Mailbox) Check(ctx context.Context) error {
	c, e := m.smtp(ctx)
	if e != nil {
		return e
	}
	c.Close()
	i, e := m.imap(ctx)
	if e != nil {
		return e
	}
	return i.Logout()
}

// Once DATA has been accepted, callers must not blindly retry an ambiguous failure.
// SMTP has no provider idempotency guarantee even when Message-ID stays stable.
func (m Mailbox) Send(ctx context.Context, message Email) error {
	if message.From != m.Email {
		return errors.New("sender must match connected mailbox")
	}
	body, e := FormatEmail(message)
	if e != nil {
		return e
	}
	c, e := m.smtp(ctx)
	if e != nil {
		return e
	}
	defer c.Close()
	from, _ := mail.ParseAddress(message.From)
	to, _ := mail.ParseAddress(message.To)
	if e = c.Mail(from.Address); e != nil {
		return e
	}
	if e = c.Rcpt(to.Address); e != nil {
		return e
	}
	w, e := c.Data()
	if e != nil {
		return e
	}
	if _, e = w.Write(body); e != nil {
		return fmt.Errorf("ambiguous SMTP delivery: %w", e)
	}
	if e = w.Close(); e != nil {
		return fmt.Errorf("ambiguous SMTP delivery: %w", e)
	}
	return nil
}
func (m Mailbox) imap(ctx context.Context) (*imapclient.Client, error) {
	if m.IMAPPort != 993 {
		return nil, errors.New("IMAP requires TLS port 993")
	}
	conn, e := dialMail(ctx, m.IMAPHost, m.IMAPPort)
	if e != nil {
		return nil, e
	}
	tc := tls.Client(conn, &tls.Config{ServerName: m.IMAPHost, MinVersion: tls.VersionTLS12})
	if e = tc.HandshakeContext(ctx); e != nil {
		conn.Close()
		return nil, e
	}
	c, e := imapclient.New(tc)
	if e != nil {
		conn.Close()
		return nil, e
	}
	user := m.Username
	if user == "" {
		user = m.Email
	}
	if m.AccessToken != "" {
		e = c.Authenticate(imapOAuth{user, m.AccessToken})
	} else {
		e = c.Login(user, m.Password)
	}
	if e != nil {
		c.Logout()
		return nil, e
	}
	return c, nil
}

type IncomingEmail struct {
	Text      string    `json:"text"`
	UID       uint32    `json:"uid"`
	MessageID string    `json:"messageId"`
	InReplyTo string    `json:"inReplyTo"`
	From      string    `json:"from"`
	Subject   string    `json:"subject"`
	Date      time.Time `json:"date"`
	Raw       string    `json:"raw"`
}
type InboxBatch struct {
	UIDValidity uint32          `json:"uidValidity"`
	Messages    []IncomingEmail `json:"messages"`
}

func (m Mailbox) Inbox(ctx context.Context, afterUID uint32) (InboxBatch, error) {
	out := InboxBatch{Messages: []IncomingEmail{}}
	c, e := m.imap(ctx)
	if e != nil {
		return out, e
	}
	defer c.Logout()
	box, e := c.Select("INBOX", true)
	if e != nil {
		return out, e
	}
	out.UIDValidity = box.UidValidity
	if box.Messages == 0 || afterUID >= box.UidNext-1 {
		return out, nil
	}
	crit := imap.NewSearchCriteria()
	crit.Uid = new(imap.SeqSet)
	crit.Uid.AddRange(afterUID+1, 0)
	ids, e := c.UidSearch(crit)
	if e != nil {
		return out, e
	}
	if len(ids) > 100 {
		ids = ids[:100]
	}
	if len(ids) == 0 {
		return out, nil
	}
	set := new(imap.SeqSet)
	set.AddNum(ids...)
	section := &imap.BodySectionName{Peek: true, Partial: []int{0, 131072}}
	messages := make(chan *imap.Message, 100)
	done := make(chan error, 1)
	go func() {
		done <- c.UidFetch(set, []imap.FetchItem{imap.FetchUid, imap.FetchEnvelope, section.FetchItem()}, messages)
	}()
	for msg := range messages {
		item := IncomingEmail{UID: msg.Uid}
		if en := msg.Envelope; en != nil {
			item.MessageID = en.MessageId
			item.InReplyTo = en.InReplyTo
			item.Subject = en.Subject
			item.Date = en.Date
			if len(en.From) > 0 {
				item.From = en.From[0].Address()
			}
		}
		if r := msg.GetBody(section); r != nil {
			b, e := io.ReadAll(io.LimitReader(r, 131072))
			if e != nil {
				return out, e
			}
			item.Raw = string(b)
			item.Text = EmailText(b)
		}
		out.Messages = append(out.Messages, item)
	}
	e = <-done
	return out, e
}
func MailOAuth(provider, clientID, secret, callback string) (*oauth2.Config, error) {
	c := &oauth2.Config{ClientID: clientID, ClientSecret: secret, RedirectURL: callback}
	if clientID == "" || secret == "" || callback == "" {
		return nil, errors.New("mail OAuth provider not configured")
	}
	switch provider {
	case "google":
		c.Endpoint = oauth2.Endpoint{AuthURL: "https://accounts.google.com/o/oauth2/v2/auth", TokenURL: "https://oauth2.googleapis.com/token"}
		c.Scopes = []string{"https://mail.google.com/"}
	case "microsoft":
		c.Endpoint = oauth2.Endpoint{AuthURL: "https://login.microsoftonline.com/common/oauth2/v2.0/authorize", TokenURL: "https://login.microsoftonline.com/common/oauth2/v2.0/token"}
		c.Scopes = []string{"offline_access", "https://outlook.office.com/IMAP.AccessAsUser.All", "https://outlook.office.com/SMTP.Send"}
	default:
		return nil, errors.New("unsupported mail OAuth provider")
	}
	return c, nil
}
func ProviderMailbox(provider, email string) Mailbox {
	m := Mailbox{Email: email, Username: email, SMTPPort: 587, IMAPPort: 993}
	switch provider {
	case "google":
		m.SMTPHost = "smtp.gmail.com"
		m.IMAPHost = "imap.gmail.com"
	case "microsoft":
		m.SMTPHost = "smtp.office365.com"
		m.IMAPHost = "outlook.office365.com"
	}
	return m
}

func EmailText(raw []byte) string {
	m, e := mail.ReadMessage(strings.NewReader(string(raw)))
	if e != nil {
		return ""
	}
	return mimeText(m.Header.Get("Content-Type"), m.Header.Get("Content-Transfer-Encoding"), m.Body, 0)
}
func mimeText(contentType, encoding string, r io.Reader, depth int) string {
	if depth > 5 {
		return ""
	}
	media, params, e := mime.ParseMediaType(contentType)
	if e != nil || media == "" {
		media = "text/plain"
	}
	if strings.HasPrefix(media, "multipart/") {
		reader := multipart.NewReader(r, params["boundary"])
		for i := 0; i < 32; i++ {
			part, e := reader.NextPart()
			if e != nil {
				break
			}
			if strings.HasPrefix(part.Header.Get("Content-Disposition"), "attachment") {
				part.Close()
				continue
			}
			text := mimeText(part.Header.Get("Content-Type"), part.Header.Get("Content-Transfer-Encoding"), part, depth+1)
			part.Close()
			if text != "" {
				return text
			}
		}
		return ""
	}
	if media != "text/plain" && media != "text/html" {
		return ""
	}
	switch strings.ToLower(encoding) {
	case "base64":
		r = base64.NewDecoder(base64.StdEncoding, r)
	case "quoted-printable":
		r = quotedprintable.NewReader(r)
	}
	b, e := io.ReadAll(io.LimitReader(r, 131072))
	if e != nil {
		return ""
	}
	if media == "text/html" {
		doc, e := html.Parse(strings.NewReader(string(b)))
		if e != nil {
			return ""
		}
		var text strings.Builder
		var walk func(*html.Node)
		walk = func(n *html.Node) {
			if n.Type == html.ElementNode && (n.Data == "script" || n.Data == "style") {
				return
			}
			if n.Type == html.TextNode {
				text.WriteString(n.Data)
				text.WriteByte(' ')
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walk(c)
			}
		}
		walk(doc)
		return strings.Join(strings.Fields(text.String()), " ")
	}
	return string(b)
}
