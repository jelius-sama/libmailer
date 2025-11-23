package api

import (
	"fmt"
	gomail "gopkg.in/gomail.v2"
	"io"
	"net/mail"
	"os"
	"strings"
)

// ParseEmailAddress handles email formats like "Name <email@domain.com>" or "email@domain.com"
func ParseEmailAddress(addr string) (string, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return "", fmt.Errorf("empty email address")
	}

	// Try parsing as RFC 5322 address
	parsed, err := mail.ParseAddress(addr)
	if err != nil {
		// If parsing fails, check if it's a simple email
		if strings.Contains(addr, "@") && !strings.Contains(addr, "<") {
			return addr, nil
		}
		return "", fmt.Errorf("invalid email address format: %w", err)
	}

	return parsed.Address, nil
}

// ParseEmailAddress handles email formats like "Name <email@domain.com>" or "email@domain.com"
func FormatEmailAddress(addr string) string {
	parsed, err := mail.ParseAddress(addr)
	if err != nil {
		return addr
	}
	return parsed.String()
}

// SendMail sends an email using provided parameters
func SendMail(smtpHost string, smtpPort int, username, password, from, to, subject, body string, cc, bcc []string, attachments []string) error {
	m := gomail.NewMessage()

	// Set From with proper formatting
	m.SetHeader("From", FormatEmailAddress(from))

	// Set To with proper formatting
	m.SetHeader("To", FormatEmailAddress(to))

	// Set CC if provided
	if len(cc) > 0 {
		formattedCC := make([]string, len(cc))
		for i, addr := range cc {
			formattedCC[i] = FormatEmailAddress(addr)
		}
		m.SetHeader("Cc", formattedCC...)
	}

	// Set BCC if provided
	if len(bcc) > 0 {
		formattedBCC := make([]string, len(bcc))
		for i, addr := range bcc {
			formattedBCC[i] = FormatEmailAddress(addr)
		}
		m.SetHeader("Bcc", formattedBCC...)
	}

	m.SetHeader("Subject", subject)

	// Detect content type (simple check for HTML)
	if strings.Contains(body, "<html") || strings.Contains(body, "<HTML") {
		m.SetBody("text/html", body)
	} else {
		m.SetBody("text/plain", body)
	}

	// Add attachments
	for _, attachment := range attachments {
		if _, err := os.Stat(attachment); err != nil {
			return fmt.Errorf("attachment not found: %s", attachment)
		}
		m.Attach(attachment)
	}

	d := gomail.NewDialer(smtpHost, smtpPort, username, password)
	return d.DialAndSend(m)
}

// SendRawEML sends a raw .eml file
func SendRawEML(smtpHost string, smtpPort int, username, password string, emlPath string) error {
	file, err := os.Open(emlPath)
	if err != nil {
		return fmt.Errorf("cannot open EML file: %w", err)
	}
	defer file.Close()

	// Parse the EML file to extract headers and body
	msg, err := mail.ReadMessage(file)
	if err != nil {
		return fmt.Errorf("invalid EML file format: %w", err)
	}

	// Create new message
	m := gomail.NewMessage()

	// Copy headers
	for key, values := range msg.Header {
		if len(values) > 0 {
			m.SetHeader(key, values...)
		}
	}

	// Read body
	bodyBytes, err := io.ReadAll(msg.Body)
	if err != nil {
		return fmt.Errorf("cannot read EML body: %w", err)
	}

	// Detect content type from header or body
	contentType := msg.Header.Get("Content-Type")
	if strings.Contains(contentType, "text/html") {
		m.SetBody("text/html", string(bodyBytes))
	} else {
		m.SetBody("text/plain", string(bodyBytes))
	}

	d := gomail.NewDialer(smtpHost, smtpPort, username, password)
	return d.DialAndSend(m)
}
