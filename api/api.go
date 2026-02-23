package api

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "net/mail"
    "os"
    "path/filepath"
    "strings"

    "github.com/aws/aws-sdk-go-v2/aws"
    awsconfig "github.com/aws/aws-sdk-go-v2/config"
    "github.com/aws/aws-sdk-go-v2/service/sesv2"
    "github.com/aws/aws-sdk-go-v2/service/sesv2/types"
    gomail "gopkg.in/gomail.v2"
)

// Config holds AWS SES configuration.
// Credentials are NOT stored here; they are resolved by the AWS SDK credential
// chain (env vars, ~/.aws/credentials, IAM role, etc.).
type Config struct {
    From         string `json:"from"`
    Region       string `json:"region"`
    UseDualStack bool   `json:"use_dual_stack,omitempty"`
}

// LoadConfigFromPath loads configuration from a specific path.
func LoadConfigFromPath(configPath string) (*Config, error) {
    data, err := os.ReadFile(configPath)
    if err != nil {
        return nil, fmt.Errorf("config file not found at %s: %w", configPath, err)
    }

    var config Config
    if err := json.Unmarshal(data, &config); err != nil {
        return nil, fmt.Errorf("invalid config file: %w", err)
    }

    if config.Region == "" {
        return nil, fmt.Errorf("invalid config file: missing required field \"region\"")
    }
    if config.From == "" {
        return nil, fmt.Errorf("invalid config file: missing required field \"from\"")
    }

    return &config, nil
}

// LoadConfig attempts to load AWS SES configuration from
// ~/.config/mailer/config.aws.json
func LoadConfig() (*Config, error) {
    homeDir, err := os.UserHomeDir()
    if err != nil {
        return nil, fmt.Errorf("cannot determine home directory: %w", err)
    }

    configPath := filepath.Join(homeDir, ".config", "mailer", "config.aws.json")
    return LoadConfigFromPath(configPath)
}

// ParseEmailAddress handles email formats like "Name <email@domain.com>" or
// "email@domain.com" and returns just the address part.
func ParseEmailAddress(addr string) (string, error) {
    addr = strings.TrimSpace(addr)
    if addr == "" {
        return "", fmt.Errorf("empty email address")
    }

    parsed, err := mail.ParseAddress(addr)
    if err != nil {
        // If parsing fails, accept a bare address without angle brackets
        if strings.Contains(addr, "@") && !strings.Contains(addr, "<") {
            return addr, nil
        }
        return "", fmt.Errorf("invalid email address format: %w", err)
    }

    return parsed.Address, nil
}

// FormatEmailAddress returns the RFC 5322 formatted address string,
// e.g. "Name <email@domain.com>". Falls back to the raw string on parse error.
func FormatEmailAddress(addr string) string {
    parsed, err := mail.ParseAddress(addr)
    if err != nil {
        return addr
    }
    return parsed.String()
}

// newSESClient constructs an SES v2 client from the provided Config.
// When UseDualStack is true the client connects over the dual-stack (IPv4+IPv6)
// endpoint, which is required for IPv6-only or IPv6-preferred AWS environments.
func newSESClient(cfg *Config) (*sesv2.Client, error) {
    awsCfg, err := awsconfig.LoadDefaultConfig(context.TODO(),
        awsconfig.WithRegion(cfg.Region),
    )
    if err != nil {
        return nil, fmt.Errorf("failed to load AWS config: %w", err)
    }

    opts := []func(*sesv2.Options){}
    if cfg.UseDualStack {
        opts = append(opts, func(o *sesv2.Options) {
            o.EndpointOptions.UseDualStackEndpoint = aws.DualStackEndpointStateEnabled
        })
    }

    return sesv2.NewFromConfig(awsCfg, opts...), nil
}

// buildMessage constructs a gomail.Message and serialises it to a raw MIME
// byte slice ready to hand directly to SES.
func buildMessage(from, to, subject, body string, cc, bcc, attachments []string) ([]byte, error) {
    m := gomail.NewMessage()

    m.SetHeader("From", FormatEmailAddress(from))
    m.SetHeader("To", FormatEmailAddress(to))

    if len(cc) > 0 {
        formattedCC := make([]string, len(cc))
        for i, addr := range cc {
            formattedCC[i] = FormatEmailAddress(addr)
        }
        m.SetHeader("Cc", formattedCC...)
    }

    if len(bcc) > 0 {
        formattedBCC := make([]string, len(bcc))
        for i, addr := range bcc {
            formattedBCC[i] = FormatEmailAddress(addr)
        }
        m.SetHeader("Bcc", formattedBCC...)
    }

    m.SetHeader("Subject", subject)

    // Detect content type (simple heuristic for HTML bodies)
    mime := http.DetectContentType([]byte(body))
    if strings.Contains(mime, "text/html") {
        m.SetBody("text/html", body)
    } else {
        m.SetBody("text/plain", body)
    }

    for _, attachment := range attachments {
        if _, err := os.Stat(attachment); err != nil {
            return nil, fmt.Errorf("attachment not found: %s", attachment)
        }
        m.Attach(attachment)
    }

    var buf bytes.Buffer
    if _, err := m.WriteTo(&buf); err != nil {
        return nil, fmt.Errorf("failed to serialise message: %w", err)
    }
    return buf.Bytes(), nil
}

// SendMail sends an email via AWS SES v2.
//
// The function signature is intentionally compatible with the original SMTP
// version so that call-sites only need to update how they build the Config —
// smtpHost, smtpPort, username and password are replaced by the single *Config
// parameter which carries the AWS region and dual-stack preference.
func SendMail(cfg *Config, from, to, subject, body string, cc, bcc []string, attachments []string) error {
    raw, err := buildMessage(from, to, subject, body, cc, bcc, attachments)
    if err != nil {
        return err
    }

    client, err := newSESClient(cfg)
    if err != nil {
        return err
    }

    _, err = client.SendEmail(context.TODO(), &sesv2.SendEmailInput{
        Content: &types.EmailContent{
            Raw: &types.RawMessage{Data: raw},
        },
    })
    return err
}

// SendRawEML sends a pre-composed .eml file via AWS SES v2.
// The file is passed verbatim as a RawMessage — no re-parsing or re-encoding
// is performed, so all original headers, encodings and MIME parts are
// preserved exactly as authored.
func SendRawEML(cfg *Config, emlPath string) error {
    data, err := os.ReadFile(emlPath)
    if err != nil {
        return fmt.Errorf("cannot open EML file: %w", err)
    }

    client, err := newSESClient(cfg)
    if err != nil {
        return err
    }

    _, err = client.SendEmail(context.TODO(), &sesv2.SendEmailInput{
        Content: &types.EmailContent{
            Raw: &types.RawMessage{Data: data},
        },
    })
    return err
}

