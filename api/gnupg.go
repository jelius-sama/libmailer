package api

import (
    "bytes"
    "crypto"
    "fmt"
    "os"
    "path/filepath"
    "strings"

    "github.com/ProtonMail/go-crypto/openpgp"
    "github.com/ProtonMail/go-crypto/openpgp/armor"
    "github.com/ProtonMail/go-crypto/openpgp/packet"
)

type signer struct {
    entity    *openpgp.Entity
    publicKey []byte // ASCII-armored, optional (attached as publickey.asc)
}

// loadSigner returns (nil, nil) when signing is not configured, so callers can
// simply send unsigned. A configured but broken key is an error: we never
// silently downgrade to an unsigned mail.
func loadSigner(cfg Config) (*signer, error) {
    if strings.TrimSpace(cfg.PrivateKeyPath) == "" {
        return nil, nil
    }

    entity, err := loadPrivateKey(cfg.PrivateKeyPath, cfg.Passphrase)
    if err != nil {
        return nil, fmt.Errorf("gpg: %w", err)
    }

    s := &signer{entity: entity}
    if strings.TrimSpace(cfg.PublicKeyPath) != "" {
        pub, err := loadPublicKey(cfg.PublicKeyPath)
        if err != nil {
            return nil, fmt.Errorf("gpg: %w", err)
        }
        s.publicKey = pub
    }
    return s, nil
}

func expanduser(path string) string {
    if strings.HasPrefix(path, "~/") {
        if home, err := os.UserHomeDir(); err == nil {
            return filepath.Join(home, path[2:])
        }
    }
    return path
}

func loadPrivateKey(path, passphrase string) (*openpgp.Entity, error) {
    data, err := os.ReadFile(expanduser(path))
    if err != nil {
        return nil, fmt.Errorf("cannot read private key: %w", err)
    }

    keyring, err := openpgp.ReadArmoredKeyRing(bytes.NewReader(data))
    if err != nil {
        keyring, err = openpgp.ReadKeyRing(bytes.NewReader(data))
        if err != nil {
            return nil, fmt.Errorf("failed to parse armored or binary key: %w", err)
        }
    }

    var entity *openpgp.Entity
    for _, e := range keyring {
        if e.PrivateKey != nil {
            entity = e
            break
        }
    }
    if entity == nil {
        return nil, fmt.Errorf("no private key found in %s", path)
    }

    if entity.PrivateKey.Encrypted {
        if passphrase == "" {
            return nil, fmt.Errorf("private key is encrypted but no passphrase is configured")
        }
        if err := entity.PrivateKey.Decrypt([]byte(passphrase)); err != nil {
            return nil, fmt.Errorf("failed to decrypt private key: %w", err)
        }
    }
    for _, sub := range entity.Subkeys {
        if sub.PrivateKey != nil && sub.PrivateKey.Encrypted {
            if err := sub.PrivateKey.Decrypt([]byte(passphrase)); err != nil {
                return nil, fmt.Errorf("failed to decrypt subkey: %w", err)
            }
        }
    }
    return entity, nil
}

func loadPublicKey(path string) ([]byte, error) {
    data, err := os.ReadFile(expanduser(path))
    if err != nil {
        return nil, fmt.Errorf("cannot read public key: %w", err)
    }
    if bytes.Contains(data, []byte("-----BEGIN PGP PUBLIC KEY BLOCK-----")) {
        return bytes.TrimSpace(data), nil
    }

    // Binary key file: armor it so it is safe as a 7bit attachment.
    var buf bytes.Buffer
    w, err := armor.Encode(&buf, "PGP PUBLIC KEY BLOCK", nil)
    if err != nil {
        return nil, err
    }
    if _, err := w.Write(data); err != nil {
        return nil, err
    }
    if err := w.Close(); err != nil {
        return nil, err
    }
    return bytes.TrimSpace(buf.Bytes()), nil
}

// micalgOf reads the hash algorithm that was really used, because the library
// may pick one according to the key preferences instead of our DefaultHash, and
// the micalg parameter must match it.
func micalgOf(sig []byte) (string, error) {
    p, err := packet.Read(bytes.NewReader(sig))
    if err != nil {
        return "", fmt.Errorf("cannot parse generated signature: %w", err)
    }
    ps, ok := p.(*packet.Signature)
    if !ok {
        return "", fmt.Errorf("unexpected packet type in generated signature")
    }
    switch ps.Hash {
    case crypto.SHA1:
        return "pgp-sha1", nil
    case crypto.SHA224:
        return "pgp-sha224", nil
    case crypto.SHA256:
        return "pgp-sha256", nil
    case crypto.SHA384:
        return "pgp-sha384", nil
    case crypto.SHA512:
        return "pgp-sha512", nil
    case crypto.RIPEMD160:
        return "pgp-ripemd160", nil
    }
    return "", fmt.Errorf("unsupported signature hash %v", ps.Hash)
}

// sign wraps a MIME entity into an RFC 3156 multipart/signed entity. The exact
// bytes of `entity` are what gets signed and what is emitted as the first part.
func (s *signer) sign(entity []byte) ([]byte, error) {
    var raw bytes.Buffer
    cfg := &packet.Config{DefaultHash: crypto.SHA256}
    if err := openpgp.DetachSignText(&raw, s.entity, bytes.NewReader(entity), cfg); err != nil {
        return nil, fmt.Errorf("gpg: signing failed: %w", err)
    }

    micalg, err := micalgOf(raw.Bytes())
    if err != nil {
        return nil, fmt.Errorf("gpg: %w", err)
    }

    var arm bytes.Buffer
    w, err := armor.Encode(&arm, "PGP SIGNATURE", nil)
    if err != nil {
        return nil, fmt.Errorf("gpg: %w", err)
    }
    if _, err := w.Write(raw.Bytes()); err != nil {
        return nil, fmt.Errorf("gpg: %w", err)
    }
    if err := w.Close(); err != nil {
        return nil, fmt.Errorf("gpg: %w", err)
    }

    boundary := newBoundary("signed")
    var out bytes.Buffer
    fmt.Fprintf(&out, "Content-Type: multipart/signed;\r\n micalg=%s;\r\n protocol=\"application/pgp-signature\";\r\n boundary=\"%s\"\r\n\r\n", micalg, boundary)
    fmt.Fprintf(&out, "--%s\r\n", boundary)
    out.Write(entity)
    fmt.Fprintf(&out, "\r\n--%s\r\n", boundary)
    out.WriteString("Content-Type: application/pgp-signature; name=\"signature.asc\"\r\n")
    out.WriteString("Content-Description: OpenPGP digital signature\r\n")
    out.WriteString("Content-Disposition: attachment; filename=\"signature.asc\"\r\n\r\n")
    out.WriteString(crlf(strings.TrimSpace(arm.String())))
    fmt.Fprintf(&out, "\r\n--%s--\r\n", boundary)
    return out.Bytes(), nil
}

