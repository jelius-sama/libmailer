package api

import (
    "encoding/json"
    "fmt"
    "net/http"
    "net/mail"
    "os"
    "path/filepath"
    "strings"
    "sync"
)

// Config mirrors ~/.config/mailer/config.json.
// The three GPG fields are optional: when private_key_path is empty every mail
// is sent unsigned, exactly like before.
type Config struct {
    Host           string `json:"host"`
    Port           int    `json:"port"`
    Username       string `json:"username"`
    Password       string `json:"password"`
    From           string `json:"from"`
    PrivateKeyPath string `json:"private_key_path"`
    PublicKeyPath  string `json:"public_key_path"`
    Passphrase     string `json:"passphrase"`
}

// The exported API has no parameter for signing settings, so the most recently
// loaded config is remembered here. If nothing was ever loaded, SendMail and
// SendRawEML quietly try the default config path.
var (
    cfgMu  sync.RWMutex
    active Config
    loaded bool
)

func activeConfig() Config {
    cfgMu.RLock()
    c, ok := active, loaded
    cfgMu.RUnlock()
    if !ok {
        if _, err := LoadConfig(); err == nil {
            cfgMu.RLock()
            c = active
            cfgMu.RUnlock()
        }
    }
    return c
}

// LoadConfigFromPath loads configuration from a specific path
func LoadConfigFromPath(configPath string) (*Config, error) {
    data, err := os.ReadFile(configPath)
    if err != nil {
        return nil, fmt.Errorf("config file not found at %s: %w", configPath, err)
    }

    var cnf Config
    if err := json.Unmarshal(data, &cnf); err != nil {
        return nil, fmt.Errorf("invalid config file: %w", err)
    }

    cfgMu.Lock()
    active, loaded = cnf, true
    cfgMu.Unlock()

    return &cnf, nil
}

// LoadConfig attempts to load configuration from ~/.config/mailer/config.json
func LoadConfig() (*Config, error) {
    homeDir, err := os.UserHomeDir()
    if err != nil {
        return nil, fmt.Errorf("cannot determine home directory: %w", err)
    }

    configPath := filepath.Join(homeDir, ".config", "mailer", "config.json")
    return LoadConfigFromPath(configPath)
}

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

// FormatEmailAddress normalises "Name <email@domain.com>" into a valid header value.
func FormatEmailAddress(addr string) string {
    parsed, err := mail.ParseAddress(addr)
    if err != nil {
        return addr
    }
    return parsed.String()
}

// parseAddressList parses one address or a comma separated list, falling back
// to the lenient single-address behaviour of ParseEmailAddress.
func parseAddressList(s string) ([]*mail.Address, error) {
    s = strings.TrimSpace(s)
    if s == "" {
        return nil, fmt.Errorf("empty email address")
    }
    if list, err := mail.ParseAddressList(s); err == nil && len(list) > 0 {
        return list, nil
    }
    addr, err := ParseEmailAddress(s)
    if err != nil {
        return nil, err
    }
    return []*mail.Address{{Address: addr}}, nil
}

func parseAddressesFlat(in []string) ([]*mail.Address, error) {
    var out []*mail.Address
    for _, s := range in {
        if strings.TrimSpace(s) == "" {
            continue
        }
        list, err := parseAddressList(s)
        if err != nil {
            return nil, err
        }
        out = append(out, list...)
    }
    return out, nil
}

// SendMail sends an email using provided parameters. The mail is GPG signed
// (RFC 3156 multipart/signed) when private_key_path is set in the config.
func SendMail(smtpHost string, smtpPort int, username, password, from, to, subject, body string, cc, bcc []string, attachments []string) error {
    fromList, err := parseAddressList(from)
    if err != nil {
        return fmt.Errorf("invalid from address: %w", err)
    }
    toList, err := parseAddressList(to)
    if err != nil {
        return fmt.Errorf("invalid to address: %w", err)
    }
    ccList, err := parseAddressesFlat(cc)
    if err != nil {
        return fmt.Errorf("invalid cc address: %w", err)
    }
    bccList, err := parseAddressesFlat(bcc)
    if err != nil {
        return fmt.Errorf("invalid bcc address: %w", err)
    }

    for _, attachment := range attachments {
        if _, err := os.Stat(attachment); err != nil {
            return fmt.Errorf("attachment not found: %s", attachment)
        }
    }

    sg, err := loadSigner(activeConfig())
    if err != nil {
        return err
    }

    // Detect content type (simple check for HTML)
    mediaType := "text/plain"
    if strings.Contains(http.DetectContentType([]byte(body)), "text/html") {
        mediaType = "text/html"
    }

    parts := [][]byte{textPart(mediaType, body)}
    for _, attachment := range attachments {
        p, err := attachmentPart(attachment)
        if err != nil {
            return err
        }
        parts = append(parts, p)
    }
    if sg != nil && len(sg.publicKey) > 0 {
        parts = append(parts, publicKeyPart(sg.publicKey))
    }

    entity := combine(parts)
    if sg != nil {
        if entity, err = sg.sign(entity); err != nil {
            return err
        }
    }

    msg := append(buildHeaders(fromList[0], toList, ccList, subject), entity...)

    var rcpts []string
    for _, list := range [][]*mail.Address{toList, ccList, bccList} {
        for _, a := range list {
            rcpts = append(rcpts, a.Address)
        }
    }

    return sendSMTP(smtpHost, smtpPort, username, password, fromList[0].Address, rcpts, msg)
}

// SendRawEML sends a raw .eml file. Headers and body are sent as they are
// (Bcc is stripped). If GPG is configured the message is signed too, unless it
// is already multipart/signed or multipart/encrypted.
func SendRawEML(smtpHost string, smtpPort int, username, password string, emlPath string) error {
    raw, err := os.ReadFile(emlPath)
    if err != nil {
        return fmt.Errorf("cannot open EML file: %w", err)
    }

    sg, err := loadSigner(activeConfig())
    if err != nil {
        return err
    }

    from, rcpts, msg, err := prepareEML(raw, sg)
    if err != nil {
        return err
    }

    return sendSMTP(smtpHost, smtpPort, username, password, from, rcpts, msg)
}

