package email

import (
	"strings"
	"testing"
)

func TestBuildMessage_stripsCRLFInHeaders(t *testing.T) {
	from := "Evil <from@example.com>\r\nBcc: attacker@evil.com>"
	to := "to@example.com\r\nCc: other@evil.com"
	subject := "Hello\r\nSubject: injected"
	body := "<p>hi</p>"

	msg := buildMessage(from, to, subject, body)

	// CRLF must not introduce additional header lines (no literal \r\n before injected field names).
	if strings.Contains(msg, "\r\nBcc:") || strings.Contains(msg, "\r\nCc:") {
		t.Fatalf("CRLF created a new header line; got:\n%s", msg)
	}
	if strings.Contains(msg, "\r\nSubject: injected") {
		t.Fatalf("CRLF created a spoof Subject line; got:\n%s", msg)
	}
	parts := strings.SplitN(msg, "\r\n\r\n", 2)
	if len(parts) != 2 {
		t.Fatalf("expected headers then blank line then body, got %d parts", len(parts))
	}
	// Stripped values are folded onto one line each (no raw CR/LF left inside header values).
	for _, line := range strings.Split(parts[0], "\r\n") {
		if strings.ContainsAny(line, "\r\n") {
			t.Fatalf("header line still contains CR/LF: %q", line)
		}
	}
}
