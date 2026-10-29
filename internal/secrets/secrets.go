// Package secrets seals values stored in the database with AES-256-GCM (SEC-4).
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const (
	keyFile    = "secret.key"
	keySize    = 32
	version    = 1
	nonceSize  = 12
	headerSize = 1 + nonceSize
)

// Box seals and opens values with the instance key.
type Box struct {
	aead        cipher.AEAD
	fingerprint string
}

// LoadOrCreate reads <dataDir>/secret.key, creating it with 32 random bytes and mode 0600
// on first start.
func LoadOrCreate(dataDir string) (*Box, error) {
	path := filepath.Join(dataDir, keyFile)
	key, err := os.ReadFile(path) //nolint:gosec // the path is the configured data directory
	if errors.Is(err, fs.ErrNotExist) {
		key = make([]byte, keySize)
		if _, err := rand.Read(key); err != nil {
			return nil, err
		}
		// O_EXCL: never overwrite a key another process created meanwhile.
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // as above
		if err != nil {
			return nil, fmt.Errorf("create secret key: %w", err)
		}
		if _, err := f.Write(key); err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("write secret key: %w", err)
		}
		if err := f.Close(); err != nil {
			return nil, fmt.Errorf("write secret key: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("read secret key: %w", err)
	}
	return New(key)
}

// New returns a Box for a 32-byte key.
func New(key []byte) (*Box, error) {
	if len(key) != keySize {
		return nil, fmt.Errorf("secret key must be %d bytes, got %d", keySize, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(key)
	return &Box{aead: aead, fingerprint: hex.EncodeToString(sum[:8])}, nil
}

// Fingerprint identifies the key without revealing it. It is stored in settings so a
// missing or replaced key file is detected at start.
func (b *Box) Fingerprint() string { return b.fingerprint }

// Seal encrypts plaintext as: 1 version byte, 12 byte random nonce, ciphertext.
func (b *Box) Seal(plaintext []byte) ([]byte, error) {
	out := make([]byte, headerSize, headerSize+len(plaintext)+b.aead.Overhead())
	out[0] = version
	if _, err := rand.Read(out[1:headerSize]); err != nil {
		return nil, err
	}
	return b.aead.Seal(out, out[1:headerSize], plaintext, out[:1]), nil
}

// Open decrypts a value produced by Seal.
func (b *Box) Open(sealed []byte) ([]byte, error) {
	if len(sealed) < headerSize+b.aead.Overhead() || sealed[0] != version {
		return nil, errors.New("secrets: malformed sealed value")
	}
	plain, err := b.aead.Open(nil, sealed[1:headerSize], sealed[headerSize:], sealed[:1])
	if err != nil {
		return nil, errors.New("secrets: cannot decrypt value (wrong key or corrupted data)")
	}
	return plain, nil
}
