package email

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"net/mail"
	"net/smtp"
	"strings"
	"time"
)

//go:embed templates/*.html
var templatesFS embed.FS

var templates *template.Template

func init() {
	templates = template.Must(template.New("").Funcs(template.FuncMap{
		"formatTime": func(t time.Time) string {
			return t.Format("Mon Jan 2, 2006 at 3:04 PM MST")
		},
		"formatDate": func(t time.Time) string {
			return t.Format("Monday, January 2, 2006")
		},
		"formatMoney": func(cents int32) string {
			return fmt.Sprintf("$%.2f", float64(cents)/100)
		},
	}).ParseFS(templatesFS, "templates/*.html"))
}

type Service struct {
	host string
	port int
	user string
	pass string
	from string
}

func New(host string, port int, user, pass, from string) *Service {
	return &Service{host: host, port: port, user: user, pass: pass, from: from}
}

type ConfirmationData struct {
	ResourceName string
	BookerName   string
	StartTime    time.Time
	EndTime      time.Time
	CancelURL    string
	ViewURL      string
}

type ReminderData struct {
	ResourceName string
	BookerName   string
	StartTime    time.Time
	EndTime      time.Time
	CancelURL    string
}

type CancellationData struct {
	ResourceName string
	BookerName   string
	StartTime    time.Time
	EndTime      time.Time
}

type ReceiptData struct {
	ResourceName     string
	BookerName       string
	StartTime        time.Time
	EndTime          time.Time
	AmountCents      int32
	StripeReceiptURL string
}

func (s *Service) SendConfirmation(to string, data ConfirmationData) error {
	return s.send(to, fmt.Sprintf("Booking Confirmed – %s", data.ResourceName), "confirmation.html", data)
}

func (s *Service) SendReminder(to string, data ReminderData) error {
	return s.send(to, fmt.Sprintf("Reminder: Your booking tomorrow – %s", data.ResourceName), "reminder.html", data)
}

func (s *Service) SendCancellation(to string, data CancellationData) error {
	return s.send(to, fmt.Sprintf("Booking Cancelled – %s", data.ResourceName), "cancellation.html", data)
}

func (s *Service) SendReceipt(to string, data ReceiptData) error {
	return s.send(to, fmt.Sprintf("Receipt – %s", data.ResourceName), "receipt.html", data)
}

func (s *Service) send(to, subject, tmplName string, data any) error {
	var buf bytes.Buffer
	if err := templates.ExecuteTemplate(&buf, tmplName, data); err != nil {
		return fmt.Errorf("email template %s: %w", tmplName, err)
	}

	msg := buildMessage(s.from, to, subject, buf.String())
	addr := fmt.Sprintf("%s:%d", s.host, s.port)

	// Use PlainAuth only when credentials are provided (skips auth for Mailpit/dev).
	var auth smtp.Auth
	if s.user != "" {
		auth = smtp.PlainAuth("", s.user, s.pass, s.host)
	}
	// smtp.SendMail's from is the envelope MAIL FROM and must be a bare addr@domain.
	// Passing "Name <addr@host>" makes the client emit invalid nested angle brackets.
	envelopeFrom := envelopeSender(s.user, s.from)
	return smtp.SendMail(addr, auth, envelopeFrom, []string{to}, []byte(msg))
}

// envelopeSender returns the RFC 5321 reverse-path for MAIL FROM.
// When SMTP credentials are set, the auth identity is used; otherwise the address
// part of SMTP_FROM is extracted (display name is only in message headers).
func envelopeSender(smtpUser, fromHeader string) string {
	if smtpUser != "" {
		return smtpUser
	}
	addr, err := mail.ParseAddress(fromHeader)
	if err == nil {
		return addr.Address
	}
	// Last resort: "Local Part <addr@host>" without strict parsing
	fromHeader = strings.TrimSpace(fromHeader)
	if i := strings.LastIndex(fromHeader, "<"); i >= 0 && strings.HasSuffix(fromHeader, ">") {
		return strings.TrimSpace(fromHeader[i+1 : len(fromHeader)-1])
	}
	return fromHeader
}

func buildMessage(from, to, subject, htmlBody string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("From: %s\r\n", from))
	sb.WriteString(fmt.Sprintf("To: %s\r\n", to))
	sb.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	sb.WriteString("MIME-Version: 1.0\r\n")
	sb.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	sb.WriteString("\r\n")
	sb.WriteString(htmlBody)
	return sb.String()
}
