package api

import (
    "crypto/tls"
    "errors"
    "fmt"
    "net"
    "net/smtp"
    "strconv"
    "strings"
    "time"
)

func isLocalhost(name string) bool {
    return name == "localhost" || name == "127.0.0.1" || name == "::1"
}

// loginAuth implements AUTH LOGIN for servers that do not offer PLAIN.
type loginAuth struct{ username, password string }

func (a *loginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
    if !server.TLS && !isLocalhost(server.Name) {
        return "", nil, errors.New("refusing to send credentials over an unencrypted connection")
    }
    return "LOGIN", nil, nil
}

func (a *loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
    if !more {
        return nil, nil
    }
    switch strings.ToLower(strings.TrimSpace(string(fromServer))) {
    case "username:":
        return []byte(a.username), nil
    case "password:":
        return []byte(a.password), nil
    }
    return nil, fmt.Errorf("unexpected LOGIN challenge %q", fromServer)
}

// sendSMTP delivers finished RFC 5322 bytes. Port 465 uses implicit TLS, every
// other port uses STARTTLS when the server offers it (587 on SES does).
func sendSMTP(host string, port int, username, password, from string, rcpts []string, msg []byte) error {
    if host == "" {
        return fmt.Errorf("smtp host is empty")
    }

    addr := net.JoinHostPort(host, strconv.Itoa(port))
    dialer := &net.Dialer{Timeout: 30 * time.Second}
    tlsCfg := &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}

    var conn net.Conn
    var err error
    if port == 465 {
        conn, err = tls.DialWithDialer(dialer, "tcp", addr, tlsCfg)
    } else {
        conn, err = dialer.Dial("tcp", addr)
    }
    if err != nil {
        return fmt.Errorf("smtp connect %s: %w", addr, err)
    }
    _ = conn.SetDeadline(time.Now().Add(5 * time.Minute))

    c, err := smtp.NewClient(conn, host)
    if err != nil {
        conn.Close()
        return fmt.Errorf("smtp handshake: %w", err)
    }
    defer c.Close()

    if port != 465 {
        if ok, _ := c.Extension("STARTTLS"); ok {
            if err := c.StartTLS(tlsCfg); err != nil {
                return fmt.Errorf("smtp STARTTLS: %w", err)
            }
        }
    }

    if username != "" {
        ok, mechs := c.Extension("AUTH")
        if !ok {
            return fmt.Errorf("smtp server does not support authentication")
        }
        mechs = strings.ToUpper(mechs)
        var auth smtp.Auth
        switch {
        case strings.Contains(mechs, "PLAIN"):
            auth = smtp.PlainAuth("", username, password, host)
        case strings.Contains(mechs, "LOGIN"):
            auth = &loginAuth{username: username, password: password}
        case strings.Contains(mechs, "CRAM-MD5"):
            auth = smtp.CRAMMD5Auth(username, password)
        default:
            return fmt.Errorf("smtp server offers no supported auth mechanism (%s)", mechs)
        }
        if err := c.Auth(auth); err != nil {
            return fmt.Errorf("smtp auth: %w", err)
        }
    }

    if err := c.Mail(from); err != nil {
        return fmt.Errorf("smtp MAIL FROM <%s>: %w", from, err)
    }
    for _, r := range rcpts {
        if err := c.Rcpt(r); err != nil {
            return fmt.Errorf("smtp RCPT TO <%s>: %w", r, err)
        }
    }

    w, err := c.Data()
    if err != nil {
        return fmt.Errorf("smtp DATA: %w", err)
    }
    if _, err := w.Write(msg); err != nil {
        _ = w.Close()
        return fmt.Errorf("smtp write: %w", err)
    }
    if err := w.Close(); err != nil {
        return fmt.Errorf("smtp message rejected: %w", err)
    }

    _ = c.Quit() // the message is already accepted at this point
    return nil
}

