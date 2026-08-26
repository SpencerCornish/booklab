package email

import (
	"bytes"
	"crypto/tls"
	"embed"
	"fmt"
	"html/template"
	"log/slog"
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
		"formatTime": func(t time.Time, tz string) string {
			if loc, err := time.LoadLocation(tz); err == nil {
				t = t.In(loc)
			}
			return t.Format("Mon Jan 2, 2006 at 3:04 PM MST")
		},
		"formatDate": func(t time.Time, tz string) string {
			if loc, err := time.LoadLocation(tz); err == nil {
				t = t.In(loc)
			}
			return t.Format("Monday, January 2, 2006")
		},
		"formatMoney": func(cents int32) string {
			return fmt.Sprintf("$%.2f", float64(cents)/100)
		},
	}).ParseFS(templatesFS, "templates/*.html"))
}

type Service struct {
	host   string
	port   int
	user   string
	pass   string
	from   string
	logger *slog.Logger
}

func New(host string, port int, user, pass, from string, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		host: host, port: port, user: user, pass: pass, from: from,
		logger: log.With("component", "email"),
	}
}

type ConfirmationData struct {
	ResourceName string
	BookerName   string
	StartTime    time.Time
	EndTime      time.Time
	CancelURL    string
	ViewURL      string
	Timezone     string
}

type ReminderData struct {
	ResourceName string
	BookerName   string
	StartTime    time.Time
	EndTime      time.Time
	CancelURL    string
	Timezone     string
}

type CancellationData struct {
	ResourceName string
	BookerName   string
	StartTime    time.Time
	EndTime      time.Time
	Timezone     string
}

type ReceiptData struct {
	ResourceName     string
	BookerName       string
	StartTime        time.Time
	EndTime          time.Time
	AmountCents      int32
	StripeReceiptURL string
	Timezone         string
}

type StaffNewBookingData struct {
	ResourceName      string
	BookerName        string
	BookerEmail       string
	Metadata          map[string]string
	StartTime         time.Time
	EndTime           time.Time
	IsReturnCustomer  bool
	PriorBookingCount int64
	AdminURL          string
	Timezone          string
}

type StaffCancellationData struct {
	ResourceName string
	BookerName   string
	BookerEmail  string
	Metadata     map[string]string
	StartTime    time.Time
	EndTime      time.Time
	AdminURL     string
	Timezone     string
}

type StaffCompletionData struct {
	ResourceName    string
	BookerName      string
	BookerEmail     string
	StartTime       time.Time
	EndTime         time.Time
	AutoAmountCents int32
	AutoChargeAt    time.Time
	AdminURL        string
	Timezone        string
}

type StaffChargeFailureData struct {
	ResourceName   string
	BookerName     string
	BookerEmail    string
	StartTime      time.Time
	EndTime        time.Time
	AmountCents    int32
	ChargeAttempts int32
	ErrorMessage   string
	Source         string // "auto" or "manual"
	AdminURL       string
	Timezone       string
}

type InterestSubmissionData struct {
	ResourceName   string
	Name           string
	Email          string
	Phone          string
	Message        string
	SelectedOption string
	SubmittedAt    time.Time
	Timezone       string
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

func (s *Service) SendStaffNewBooking(to string, data StaffNewBookingData) error {
	return s.send(to, fmt.Sprintf("New Booking – %s", data.ResourceName), "staff_new_booking.html", data)
}

func (s *Service) SendStaffCancellation(to string, data StaffCancellationData) error {
	return s.send(to, fmt.Sprintf("Booking Cancelled – %s", data.ResourceName), "staff_cancellation.html", data)
}

func (s *Service) SendStaffCompletion(to string, data StaffCompletionData) error {
	return s.send(to, fmt.Sprintf("Session Complete – Action Required – %s", data.ResourceName), "staff_completion.html", data)
}

func (s *Service) SendStaffChargeFailure(to string, data StaffChargeFailureData) error {
	return s.send(to, fmt.Sprintf("⚠️ Charge Failed – %s – %s", data.BookerName, data.ResourceName), "staff_charge_failure.html", data)
}

func (s *Service) SendInterestSubmission(to string, data InterestSubmissionData) error {
	return s.send(to, fmt.Sprintf("Interest Submission – %s – %s", data.Name, data.ResourceName), "interest_submission.html", data)
}

func (s *Service) send(to, subject, tmplName string, data any) error {
	var buf bytes.Buffer
	if err := templates.ExecuteTemplate(&buf, tmplName, data); err != nil {
		s.logger.Error("email template render failed", "template", tmplName, "recipient", to, "error", err)
		return fmt.Errorf("email template %s: %w", tmplName, err)
	}

	msg := buildMessage(s.from, to, subject, buf.String())
	addr := fmt.Sprintf("%s:%d", s.host, s.port)

	// Use PlainAuth only when credentials are provided (skips auth for Mailpit/dev).
	var auth smtp.Auth
	if s.user != "" {
		auth = smtp.PlainAuth("", s.user, s.pass, s.host)
	}
	s.logger.Info("sending email", "template", tmplName, "smtp_host", addr, "recipient", to)
	if err := s.sendMail(addr, auth, envelopeAddr(s.from), []string{to}, []byte(msg)); err != nil {
		s.logger.Error("email send failed", "template", tmplName, "smtp_host", addr, "recipient", to, "error", err)
		return err
	}
	s.logger.Info("email sent", "template", tmplName, "recipient", to)
	return nil
}

// sendMail sends an email, using implicit TLS when the port is 465 or 2465
// and STARTTLS (via smtp.SendMail) for all other ports (e.g. 587).
func (s *Service) sendMail(addr string, auth smtp.Auth, from string, to []string, msg []byte) error {
	if s.port == 465 || s.port == 2465 {
		tlsCfg := &tls.Config{ServerName: s.host}
		conn, err := tls.Dial("tcp", addr, tlsCfg)
		if err != nil {
			return fmt.Errorf("tls dial: %w", err)
		}
		defer conn.Close()
		c, err := smtp.NewClient(conn, s.host)
		if err != nil {
			return fmt.Errorf("smtp new client: %w", err)
		}
		defer c.Close()
		if auth != nil {
			if err := c.Auth(auth); err != nil {
				return fmt.Errorf("smtp auth: %w", err)
			}
		}
		if err := c.Mail(from); err != nil {
			return fmt.Errorf("smtp MAIL FROM: %w", err)
		}
		for _, r := range to {
			if err := c.Rcpt(r); err != nil {
				return fmt.Errorf("smtp RCPT TO %s: %w", r, err)
			}
		}
		w, err := c.Data()
		if err != nil {
			return fmt.Errorf("smtp DATA: %w", err)
		}
		if _, err := w.Write(msg); err != nil {
			return fmt.Errorf("smtp write body: %w", err)
		}
		return w.Close()
	}
	return smtp.SendMail(addr, auth, from, to, msg)
}


// envelopeAddr extracts the bare email address from a From value that may
// include a display name (e.g. "BookLab <noreply@example.com>"). The SMTP
// MAIL FROM command accepts only a bare address; display names are only valid
// inside message headers.
func envelopeAddr(from string) string {
	if addr, err := mail.ParseAddress(from); err == nil {
		return addr.Address
	}
	return from
}

func sanitizeHeader(s string) string {
	return strings.NewReplacer("\r", "", "\n", "").Replace(s)
}

func buildMessage(from, to, subject, htmlBody string) string {
	from = sanitizeHeader(from)
	to = sanitizeHeader(to)
	subject = sanitizeHeader(subject)
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
