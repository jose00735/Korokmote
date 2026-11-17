// Package pairing manages the daemon identity, paired devices and pairing windows.
package pairing

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

// Identity is the daemon's stable ID and TLS identity.
type Identity struct {
	ID          string
	Certificate tls.Certificate
	Fingerprint string // lowercase hex SHA-256 of the leaf certificate (DER)
}

type identityFile struct {
	ID string `json:"id"`
}

// LoadIdentity loads or creates identity.json, cert.pem and key.pem in dir.
func LoadIdentity(dir string) (*Identity, error) {
	idPath := filepath.Join(dir, "identity.json")
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")

	var idf identityFile
	if b, err := os.ReadFile(idPath); err == nil {
		_ = json.Unmarshal(b, &idf)
	}
	if idf.ID == "" {
		idf.ID = NewUUID()
		b, _ := json.MarshalIndent(idf, "", "  ")
		if err := os.WriteFile(idPath, b, 0o600); err != nil {
			return nil, fmt.Errorf("write identity: %w", err)
		}
	}

	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			// Corrupt or partial files: regenerate. Paired devices will need to pair again.
			_ = os.Remove(certPath)
			_ = os.Remove(keyPath)
		}
		if err := generateCert(certPath, keyPath, idf.ID); err != nil {
			return nil, err
		}
		cert, err = tls.LoadX509KeyPair(certPath, keyPath)
		if err != nil {
			return nil, fmt.Errorf("load generated certificate: %w", err)
		}
	}
	sum := sha256.Sum256(cert.Certificate[0])
	return &Identity{ID: idf.ID, Certificate: cert, Fingerprint: hex.EncodeToString(sum[:])}, nil
}

func generateCert(certPath, keyPath, id string) error {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("generate key: %w", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	if err != nil {
		return fmt.Errorf("serial: %w", err)
	}
	tpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "korokmote-" + id, Organization: []string{"Korokmote"}},
		NotBefore:             time.Now().Add(-24 * time.Hour),
		NotAfter:              time.Now().AddDate(20, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"korokmote.local"},
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		return fmt.Errorf("create certificate: %w", err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return fmt.Errorf("marshal key: %w", err)
	}
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644); err != nil {
		return fmt.Errorf("write cert: %w", err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		return fmt.Errorf("write key: %w", err)
	}
	return nil
}

// NewUUID returns a random RFC 4122 version 4 UUID.
func NewUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}
