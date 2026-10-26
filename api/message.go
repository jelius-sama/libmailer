package api

import (
    "bytes"
    "crypto/rand"
    "encoding/base64"
    "encoding/hex"
    "fmt"
    "mime"
    "mime/quotedprintable"
    "net/mail"
    "os"
    "path/filepath"
    "strings"
    "time"
)

// A "part" / "entity" below is a complete MIME entity: its own headers, a blank
// line, then the body. Entities never end with a forced CRLF.

func crlf(s string) string {
    return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\n", "\r\n")
}

func randHex(n int) string {
    b := make([]byte, n)
    _, _ = rand.Read(b)
    return hex.EncodeToString(b)
}

func newBoundary(kind string) string {
    return "libmailer-" + kind + "-" + randHex(12)
}

func isASCII(b []byte) bool {
    for _, c := range b {
        if c >= 0x80 {
            return false
        }
    }
    return true
}

func qpEncode(p []byte) []byte {
    var b bytes.Buffer
    w := quotedprintable.NewWriter(&b)
    _, _ = w.Write(p)
    _ = w.Close()
    return b.Bytes()
}

func textPart(mediaType, body string) []byte {
    var b bytes.Buffer
    fmt.Fprintf(&b, "Content-Type: %s; charset=UTF-8\r\n", mediaType)
    b.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
    b.Write(qpEncode([]byte(crlf(body))))
    return b.Bytes()
}

func paramHeader(mediaType, key, val string) string {
    if s := mime.FormatMediaType(mediaType, map[string]string{key: val}); s != "" {
        return s
    }
    return mediaType
}

func attachmentPart(path string) ([]byte, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, fmt.Errorf("cannot read attachment %s: %w", path, err)
    }

    name := filepath.Base(path)
    ct := "application/octet-stream"
    if t := mime.TypeByExtension(filepath.Ext(name)); t != "" {
        if mt, _, err := mime.ParseMediaType(t); err == nil {
            ct = mt
        }
    }

    var b bytes.Buffer
    b.WriteString("Content-Type: " + paramHeader(ct, "name", name) + "\r\n")
    b.WriteString("Content-Disposition: " + paramHeader("attachment", "filename", name) + "\r\n")
    b.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
    enc := base64.StdEncoding.EncodeToString(data)
    for len(enc) > 76 {
        b.WriteString(enc[:76] + "\r\n")
        enc = enc[76:]
    }
    b.WriteString(enc)
    return b.Bytes(), nil
}

func publicKeyPart(armored []byte) []byte {
    var b bytes.Buffer
    b.WriteString("Content-Type: application/pgp-keys; name=\"publickey.asc\"\r\n")
    b.WriteString("Content-Disposition: attachment; filename=\"publickey.asc\"\r\n")
    b.WriteString("Content-Transfer-Encoding: 7bit\r\n\r\n")
    b.WriteString(crlf(strings.TrimSpace(string(armored))))
    return b.Bytes()
}

func mixedEntity(parts [][]byte) []byte {
    boundary := newBoundary("mixed")
    var e bytes.Buffer
    fmt.Fprintf(&e, "Content-Type: multipart/mixed; boundary=\"%s\"\r\n\r\n", boundary)
    for _, p := range parts {
        fmt.Fprintf(&e, "--%s\r\n", boundary)
        e.Write(p)
        e.WriteString("\r\n")
    }
    fmt.Fprintf(&e, "--%s--\r\n", boundary)
    return e.Bytes()
}

func combine(parts [][]byte) []byte {
    if len(parts) == 1 {
        return parts[0]
    }
    return mixedEntity(parts)
}

func newMessageID(fromAddr string) string {
    domain := "localhost"
    if i := strings.LastIndex(fromAddr, "@"); i >= 0 && i+1 < len(fromAddr) {
        domain = fromAddr[i+1:]
    }
    return "<" + randHex(16) + "@" + domain + ">"
}

func writeAddrHeader(b *bytes.Buffer, name string, addrs []*mail.Address) {
    line := name + ":"
    for i, a := range addrs {
        piece := a.String()
        if i < len(addrs)-1 {
            piece += ","
        }
        if i > 0 && len(line)+1+len(piece) > 76 {
            b.WriteString(line + "\r\n")
            line = " " + piece
        } else {
            line += " " + piece
        }
    }
    b.WriteString(line + "\r\n")
}

// buildHeaders writes the top-level headers (everything except the Content-*
// headers, which belong to the entity that follows). Bcc is never written.
func buildHeaders(from *mail.Address, to, cc []*mail.Address, subject string) []byte {
    var b bytes.Buffer
    b.WriteString("From: " + from.String() + "\r\n")
    writeAddrHeader(&b, "To", to)
    if len(cc) > 0 {
        writeAddrHeader(&b, "Cc", cc)
    }
    subject = strings.NewReplacer("\r", " ", "\n", " ").Replace(subject)
    b.WriteString("Subject: " + mime.QEncoding.Encode("UTF-8", subject) + "\r\n")
    b.WriteString("Date: " + time.Now().Format(time.RFC1123Z) + "\r\n")
    b.WriteString("Message-ID: " + newMessageID(from.Address) + "\r\n")
    b.WriteString("MIME-Version: 1.0\r\n")
    return b.Bytes()
}

// ---------------------------------------------------------------------------
// Raw .eml handling
// ---------------------------------------------------------------------------

type headerField struct {
    name string
    raw  string // the field exactly as written, folds included, ending in CRLF
}

func splitEML(data []byte) ([]headerField, []byte) {
    var head, body []byte
    if i := bytes.Index(data, []byte("\r\n\r\n")); i >= 0 {
        head, body = data[:i], data[i+4:]
    } else {
        head = bytes.TrimRight(data, "\r\n")
    }

    var fields []headerField
    for _, line := range strings.Split(string(head), "\r\n") {
        if line == "" {
            continue
        }
        if (line[0] == ' ' || line[0] == '\t') && len(fields) > 0 {
            fields[len(fields)-1].raw += line + "\r\n"
            continue
        }
        c := strings.IndexByte(line, ':')
        if c <= 0 {
            continue // e.g. an mbox "From " line
        }
        fields = append(fields, headerField{name: strings.TrimSpace(line[:c]), raw: line + "\r\n"})
    }
    return fields, body
}

func headerValues(fields []headerField, name string) []string {
    var out []string
    for _, f := range fields {
        if strings.EqualFold(f.name, name) {
            v := f.raw[strings.IndexByte(f.raw, ':')+1:]
            out = append(out, strings.TrimSpace(strings.ReplaceAll(v, "\r\n", "")))
        }
    }
    return out
}

func hasField(fields []headerField, name string) bool {
    return len(headerValues(fields, name)) > 0
}

func missingHeaders(fields []headerField, fromAddr string) string {
    var s string
    if !hasField(fields, "date") {
        s += "Date: " + time.Now().Format(time.RFC1123Z) + "\r\n"
    }
    if !hasField(fields, "message-id") {
        s += "Message-ID: " + newMessageID(fromAddr) + "\r\n"
    }
    return s
}

// emlEntity rebuilds the signable entity from the Content-* headers and the body
// of an .eml. Non-ASCII bodies without an encoding are quoted-printable encoded
// so the signature survives transport.
func emlEntity(ent []headerField, body []byte) []byte {
    ct, cte := "", ""
    if v := headerValues(ent, "content-type"); len(v) > 0 {
        ct = strings.ToLower(v[0])
    }
    if v := headerValues(ent, "content-transfer-encoding"); len(v) > 0 {
        cte = strings.ToLower(v[0])
    }

    if ct == "" {
        ent = append(ent, headerField{name: "Content-Type", raw: "Content-Type: text/plain; charset=UTF-8\r\n"})
        ct = "text/plain"
    }

    if !strings.HasPrefix(ct, "multipart/") && !isASCII(body) &&
        (cte == "" || cte == "7bit" || cte == "8bit" || cte == "binary") {
        var kept []headerField
        for _, f := range ent {
            if !strings.EqualFold(f.name, "content-transfer-encoding") {
                kept = append(kept, f)
            }
        }
        ent = append(kept, headerField{name: "Content-Transfer-Encoding", raw: "Content-Transfer-Encoding: quoted-printable\r\n"})
        body = qpEncode(body)
    }

    var b bytes.Buffer
    for _, f := range ent {
        b.WriteString(f.raw)
    }
    b.WriteString("\r\n")
    b.Write(body)
    return b.Bytes()
}

// prepareEML returns the envelope sender, envelope recipients and final bytes.
func prepareEML(raw []byte, sg *signer) (string, []string, []byte, error) {
    fields, body := splitEML([]byte(crlf(string(raw))))
    if len(fields) == 0 {
        return "", nil, nil, fmt.Errorf("invalid EML file format: no headers found")
    }

    fromVals := headerValues(fields, "from")
    if len(fromVals) == 0 {
        return "", nil, nil, fmt.Errorf("invalid EML file format: missing From header")
    }
    fromList, err := parseAddressList(fromVals[0])
    if err != nil {
        return "", nil, nil, fmt.Errorf("invalid From header: %w", err)
    }
    from := fromList[0].Address

    var rcpts []string
    for _, h := range []string{"to", "cc", "bcc"} {
        for _, v := range headerValues(fields, h) {
            if v == "" {
                continue
            }
            list, err := parseAddressList(v)
            if err != nil {
                return "", nil, nil, fmt.Errorf("invalid %s header: %w", h, err)
            }
            for _, a := range list {
                rcpts = append(rcpts, a.Address)
            }
        }
    }
    if len(rcpts) == 0 {
        return "", nil, nil, fmt.Errorf("EML file has no recipients")
    }

    ct := ""
    if v := headerValues(fields, "content-type"); len(v) > 0 {
        ct = strings.ToLower(v[0])
    }
    secured := strings.HasPrefix(ct, "multipart/signed") || strings.HasPrefix(ct, "multipart/encrypted")

    // Unsigned path (no GPG configured, or the file is already signed/encrypted)
    if sg == nil || secured {
        var out bytes.Buffer
        out.WriteString(missingHeaders(fields, from))
        if !hasField(fields, "mime-version") {
            out.WriteString("MIME-Version: 1.0\r\n")
        }
        for _, f := range fields {
            if !strings.EqualFold(f.name, "bcc") {
                out.WriteString(f.raw)
            }
        }
        out.WriteString("\r\n")
        out.Write(body)
        return from, rcpts, out.Bytes(), nil
    }

    // Signed path: Content-* headers + body become the signed entity, every
    // other header stays at the top level.
    var top bytes.Buffer
    var ent []headerField
    for _, f := range fields {
        n := strings.ToLower(f.name)
        switch {
        case n == "bcc", n == "mime-version", n == "content-length":
            // dropped
        case strings.HasPrefix(n, "content-"):
            ent = append(ent, f)
        default:
            top.WriteString(f.raw)
        }
    }
    top.WriteString(missingHeaders(fields, from))
    top.WriteString("MIME-Version: 1.0\r\n")

    entity := emlEntity(ent, body)
    if len(sg.publicKey) > 0 {
        entity = combine([][]byte{entity, publicKeyPart(sg.publicKey)})
    }
    signed, err := sg.sign(entity)
    if err != nil {
        return "", nil, nil, err
    }

    return from, rcpts, append(top.Bytes(), signed...), nil
}

