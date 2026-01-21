// Copyright 2024 The Obsidian Authors
// This file is part of the Obsidian library.

package keystore

import (
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/google/uuid"
)

// Account represents an Obsidian account
type Account struct {
	Address common.Address `json:"address"`
	URL     URL            `json:"url"`
}

// URL represents the location of a keystore file
type URL struct {
	Scheme string `json:"scheme"` // e.g., "keystore"
	Path   string `json:"path"`   // file path
}

// String returns the string representation of the URL
func (u URL) String() string {
	if u.Scheme != "" {
		return fmt.Sprintf("%s://%s", u.Scheme, u.Path)
	}
	return u.Path
}

// encryptedKeyJSON is the format of encrypted key files
type encryptedKeyJSON struct {
	Address string     `json:"address"`
	ID      string     `json:"id"`
	Version int        `json:"version"`
	Crypto  cryptoJSON `json:"crypto"`
}

type cryptoJSON struct {
	Cipher       string                 `json:"cipher"`
	CipherParams cipherparamsJSON       `json:"cipherparams"`
	CipherText   string                 `json:"ciphertext"`
	KDF          string                 `json:"kdf"`
	KDFParams    map[string]interface{} `json:"kdfparams"`
	MAC          string                 `json:"mac"`
}

type cipherparamsJSON struct {
	IV string `json:"iv"`
}

// Key represents an unencrypted private key
type Key struct {
	ID         uuid.UUID
	Address    common.Address
	PrivateKey *ecdsa.PrivateKey
}

// NewKey generates a new key
func NewKey() (*Key, error) {
	privateKey, err := crypto.GenerateKey()
	if err != nil {
		return nil, err
	}

	return &Key{
		ID:         uuid.New(),
		Address:    crypto.PubkeyToAddress(privateKey.PublicKey),
		PrivateKey: privateKey,
	}, nil
}

// NewKeyFromECDSA creates a key from an existing private key
func NewKeyFromECDSA(privateKey *ecdsa.PrivateKey) *Key {
	return &Key{
		ID:         uuid.New(),
		Address:    crypto.PubkeyToAddress(privateKey.PublicKey),
		PrivateKey: privateKey,
	}
}

// NewKeyFromHex creates a key from a hex-encoded private key
func NewKeyFromHex(hexKey string) (*Key, error) {
	privateKey, err := crypto.HexToECDSA(hexKey)
	if err != nil {
		return nil, err
	}
	return NewKeyFromECDSA(privateKey), nil
}

// StoreKey encrypts and stores a key to a file
func StoreKey(dir string, key *Key, password string) (string, error) {
	// Encrypt the key
	encryptedKey, err := EncryptKey(key, password)
	if err != nil {
		return "", err
	}

	// Marshal to JSON
	content, err := json.MarshalIndent(encryptedKey, "", "  ")
	if err != nil {
		return "", err
	}

	// Create filename
	filename := keyFileName(key.Address)
	path := filepath.Join(dir, filename)

	// Ensure directory exists
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}

	// Write file
	if err := os.WriteFile(path, content, 0600); err != nil {
		return "", err
	}

	return path, nil
}

// LoadKey loads and decrypts a key from a file
func LoadKey(path string, password string) (*Key, error) {
	// Read file
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Parse JSON
	var encryptedKey encryptedKeyJSON
	if err := json.Unmarshal(content, &encryptedKey); err != nil {
		return nil, err
	}

	// Decrypt
	return DecryptKey(&encryptedKey, password)
}

// keyFileName generates a keystore filename
func keyFileName(address common.Address) string {
	ts := time.Now().UTC()
	return fmt.Sprintf("UTC--%s--%s",
		ts.Format("2006-01-02T15-04-05.999999999Z"),
		hex.EncodeToString(address[:]))
}

// WriteTemporaryKeyFile writes a key to a temporary file
func WriteTemporaryKeyFile(file string, content []byte) (string, error) {
	// Create the keystore directory with appropriate permissions
	const dirPerm = 0700
	if err := os.MkdirAll(filepath.Dir(file), dirPerm); err != nil {
		return "", err
	}

	// Atomic write: create a temporary file, write content, rename
	f, err := os.CreateTemp(filepath.Dir(file), "."+filepath.Base(file)+".tmp")
	if err != nil {
		return "", err
	}
	defer f.Close()

	if _, err := f.Write(content); err != nil {
		os.Remove(f.Name())
		return "", err
	}

	// Sync to ensure the content is written
	if err := f.Sync(); err != nil {
		os.Remove(f.Name())
		return "", err
	}

	// Close the file before renaming
	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		return "", err
	}

	// Rename to final filename
	if err := os.Rename(f.Name(), file); err != nil {
		os.Remove(f.Name())
		return "", err
	}

	return file, nil
}

// GenerateRandomBytes generates random bytes using crypto/rand
func GenerateRandomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := io.ReadFull(rand.Reader, b)
	return b, err
}
