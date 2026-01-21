// Copyright 2024 The Obsidian Authors
// This file is part of the Obsidian library.

package keystore

import (
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	obstypes "github.com/obsidian-chain/obsidian/core/types"
)

var (
	ErrNoMatch      = errors.New("no key for given address")
	ErrLocked       = errors.New("account is locked")
	ErrAlreadyExist = errors.New("account already exists")
)

// unlockState tracks the unlock state of an account
type unlockState struct {
	key    *Key
	expiry time.Time // zero means indefinitely unlocked
}

// KeyStore manages encrypted key files
type KeyStore struct {
	keyDir  string
	scryptN int
	scryptP int

	mu       sync.RWMutex
	accounts map[common.Address]*accountCache
	unlocked map[common.Address]*unlockState

	quit chan struct{}
}

// accountCache caches account information
type accountCache struct {
	Account Account
	path    string
}

// NewKeyStore creates a new keystore
func NewKeyStore(keyDir string) *KeyStore {
	ks := &KeyStore{
		keyDir:   keyDir,
		scryptN:  scryptN,
		scryptP:  scryptP,
		accounts: make(map[common.Address]*accountCache),
		unlocked: make(map[common.Address]*unlockState),
		quit:     make(chan struct{}),
	}

	// Scan existing keys
	ks.scanAccounts()

	// Start expiration loop
	go ks.expireLoop()

	return ks
}

// NewLightKeyStore creates a keystore with lighter scrypt parameters (for testing)
func NewLightKeyStore(keyDir string) *KeyStore {
	ks := &KeyStore{
		keyDir:   keyDir,
		scryptN:  lightScryptN,
		scryptP:  lightScryptP,
		accounts: make(map[common.Address]*accountCache),
		unlocked: make(map[common.Address]*unlockState),
		quit:     make(chan struct{}),
	}

	ks.scanAccounts()
	go ks.expireLoop()
	return ks
}

// Close stops the keystore
func (ks *KeyStore) Close() {
	close(ks.quit)
}

// expireLoop periodically checks for and removes expired unlocks
func (ks *KeyStore) expireLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ks.expireUnlocked()
		case <-ks.quit:
			return
		}
	}
}

// expireUnlocked removes accounts whose unlock has expired
func (ks *KeyStore) expireUnlocked() {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	now := time.Now()
	for addr, state := range ks.unlocked {
		if !state.expiry.IsZero() && now.After(state.expiry) {
			delete(ks.unlocked, addr)
			log.Info("Account auto-locked due to timeout", "address", addr.Hex())
		}
	}
}

// scanAccounts scans the keystore directory for existing accounts
func (ks *KeyStore) scanAccounts() {
	if ks.keyDir == "" {
		return
	}

	files, err := os.ReadDir(ks.keyDir)
	if err != nil {
		log.Debug("Failed to read keystore directory", "dir", ks.keyDir, "err", err)
		return
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		path := filepath.Join(ks.keyDir, file.Name())

		// Try to parse the file as a key file
		key, err := LoadKey(path, "")
		if err != nil {
			// Try to at least get the address from the filename
			// Format: UTC--<date>--<address>
			if len(file.Name()) >= 40 {
				addrHex := file.Name()[len(file.Name())-40:]
				addr := common.HexToAddress(addrHex)
				ks.accounts[addr] = &accountCache{
					Account: Account{
						Address: addr,
						URL: URL{
							Scheme: "keystore",
							Path:   path,
						},
					},
					path: path,
				}
			}
			continue
		}

		ks.accounts[key.Address] = &accountCache{
			Account: Account{
				Address: key.Address,
				URL: URL{
					Scheme: "keystore",
					Path:   path,
				},
			},
			path: path,
		}
	}

	log.Info("Keystore loaded", "accounts", len(ks.accounts))
}

// Accounts returns all accounts in the keystore
func (ks *KeyStore) Accounts() []Account {
	ks.mu.RLock()
	defer ks.mu.RUnlock()

	accounts := make([]Account, 0, len(ks.accounts))
	for _, cache := range ks.accounts {
		accounts = append(accounts, cache.Account)
	}
	return accounts
}

// HasAddress checks if an address is in the keystore
func (ks *KeyStore) HasAddress(addr common.Address) bool {
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	_, exists := ks.accounts[addr]
	return exists
}

// Find returns the account for the given address
func (ks *KeyStore) Find(addr common.Address) (Account, error) {
	ks.mu.RLock()
	defer ks.mu.RUnlock()

	cache, exists := ks.accounts[addr]
	if !exists {
		return Account{}, ErrNoMatch
	}
	return cache.Account, nil
}

// NewAccount creates a new account with the given password
func (ks *KeyStore) NewAccount(password string) (Account, error) {
	key, err := NewKey()
	if err != nil {
		return Account{}, err
	}

	path, err := StoreKey(ks.keyDir, key, password)
	if err != nil {
		return Account{}, err
	}

	account := Account{
		Address: key.Address,
		URL: URL{
			Scheme: "keystore",
			Path:   path,
		},
	}

	ks.mu.Lock()
	ks.accounts[key.Address] = &accountCache{
		Account: account,
		path:    path,
	}
	ks.mu.Unlock()

	log.Info("New account created", "address", key.Address.Hex())
	return account, nil
}

// Import imports an existing private key
func (ks *KeyStore) Import(privateKey *ecdsa.PrivateKey, password string) (Account, error) {
	key := NewKeyFromECDSA(privateKey)

	ks.mu.Lock()
	if _, exists := ks.accounts[key.Address]; exists {
		ks.mu.Unlock()
		return Account{}, ErrAlreadyExist
	}
	ks.mu.Unlock()

	path, err := StoreKey(ks.keyDir, key, password)
	if err != nil {
		return Account{}, err
	}

	account := Account{
		Address: key.Address,
		URL: URL{
			Scheme: "keystore",
			Path:   path,
		},
	}

	ks.mu.Lock()
	ks.accounts[key.Address] = &accountCache{
		Account: account,
		path:    path,
	}
	ks.mu.Unlock()

	log.Info("Account imported", "address", key.Address.Hex())
	return account, nil
}

// ImportHex imports a hex-encoded private key
func (ks *KeyStore) ImportHex(hexKey string, password string) (Account, error) {
	privateKey, err := crypto.HexToECDSA(hexKey)
	if err != nil {
		return Account{}, err
	}
	return ks.Import(privateKey, password)
}

// Export exports an account's encrypted key file
func (ks *KeyStore) Export(addr common.Address, password, newPassword string) ([]byte, error) {
	ks.mu.RLock()
	cache, exists := ks.accounts[addr]
	ks.mu.RUnlock()

	if !exists {
		return nil, ErrNoMatch
	}

	// Load and decrypt the key
	key, err := LoadKey(cache.path, password)
	if err != nil {
		return nil, err
	}

	// Re-encrypt with new password
	encryptedKey, err := EncryptKey(key, newPassword)
	if err != nil {
		return nil, err
	}

	return encryptedKey.marshal()
}

// Delete deletes an account from the keystore
func (ks *KeyStore) Delete(addr common.Address, password string) error {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	cache, exists := ks.accounts[addr]
	if !exists {
		return ErrNoMatch
	}

	// Verify password by trying to load the key
	_, err := LoadKey(cache.path, password)
	if err != nil {
		return err
	}

	// Remove from unlocked if present
	delete(ks.unlocked, addr)

	// Delete the file
	if err := os.Remove(cache.path); err != nil {
		return err
	}

	// Remove from accounts
	delete(ks.accounts, addr)

	log.Info("Account deleted", "address", addr.Hex())
	return nil
}

// TimedUnlock unlocks an account with the given password for a specified duration
// If duration is 0, the account remains unlocked indefinitely
func (ks *KeyStore) TimedUnlock(addr common.Address, password string, duration time.Duration) error {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	cache, exists := ks.accounts[addr]
	if !exists {
		return ErrNoMatch
	}

	// If already unlocked with no expiry or longer expiry, keep it
	if state, unlocked := ks.unlocked[addr]; unlocked {
		if state.expiry.IsZero() {
			return nil // Already unlocked indefinitely
		}
		if duration == 0 {
			// Upgrading to indefinite unlock
			state.expiry = time.Time{}
			return nil
		}
		newExpiry := time.Now().Add(duration)
		if newExpiry.After(state.expiry) {
			state.expiry = newExpiry
		}
		return nil
	}

	// Load and decrypt the key
	key, err := LoadKey(cache.path, password)
	if err != nil {
		return err
	}

	var expiry time.Time
	if duration > 0 {
		expiry = time.Now().Add(duration)
	}

	ks.unlocked[addr] = &unlockState{
		key:    key,
		expiry: expiry,
	}

	if duration > 0 {
		log.Info("Account unlocked", "address", addr.Hex(), "duration", duration)
	} else {
		log.Info("Account unlocked indefinitely", "address", addr.Hex())
	}
	return nil
}

// Unlock unlocks an account indefinitely (for backward compatibility)
func (ks *KeyStore) Unlock(addr common.Address, password string) error {
	return ks.TimedUnlock(addr, password, 0)
}

// Lock locks an account
func (ks *KeyStore) Lock(addr common.Address) error {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	if _, exists := ks.unlocked[addr]; !exists {
		return ErrNoMatch
	}

	delete(ks.unlocked, addr)
	log.Info("Account locked", "address", addr.Hex())
	return nil
}

// IsUnlocked checks if an account is unlocked
func (ks *KeyStore) IsUnlocked(addr common.Address) bool {
	ks.mu.RLock()
	defer ks.mu.RUnlock()

	state, exists := ks.unlocked[addr]
	if !exists {
		return false
	}
	// Check if expired
	if !state.expiry.IsZero() && time.Now().After(state.expiry) {
		return false
	}
	return true
}

// getUnlockedKey returns the key if unlocked and not expired
func (ks *KeyStore) getUnlockedKey(addr common.Address) (*Key, error) {
	state, exists := ks.unlocked[addr]
	if !exists {
		return nil, ErrLocked
	}
	// Check if expired
	if !state.expiry.IsZero() && time.Now().After(state.expiry) {
		return nil, ErrLocked
	}
	return state.key, nil
}

// SignHash signs a hash with an unlocked account
func (ks *KeyStore) SignHash(addr common.Address, hash []byte) ([]byte, error) {
	ks.mu.RLock()
	key, err := ks.getUnlockedKey(addr)
	ks.mu.RUnlock()

	if err != nil {
		return nil, err
	}

	return crypto.Sign(hash, key.PrivateKey)
}

// SignTx signs a transaction with an unlocked account
func (ks *KeyStore) SignTx(addr common.Address, tx *obstypes.StealthTransaction, chainID *big.Int) (*obstypes.StealthTransaction, error) {
	ks.mu.RLock()
	key, err := ks.getUnlockedKey(addr)
	ks.mu.RUnlock()

	if err != nil {
		return nil, err
	}

	signer := obstypes.NewStealthEIP155Signer(chainID)
	txHash := signer.Hash(tx)
	sig, err := crypto.Sign(txHash[:], key.PrivateKey)
	if err != nil {
		return nil, err
	}
	return tx.WithSignature(signer, sig)
}

// SignTxWithPassword signs a transaction without unlocking the account
func (ks *KeyStore) SignTxWithPassword(addr common.Address, password string, tx *obstypes.StealthTransaction, chainID *big.Int) (*obstypes.StealthTransaction, error) {
	ks.mu.RLock()
	cache, exists := ks.accounts[addr]
	ks.mu.RUnlock()

	if !exists {
		return nil, ErrNoMatch
	}

	key, err := LoadKey(cache.path, password)
	if err != nil {
		return nil, err
	}

	signer := obstypes.NewStealthEIP155Signer(chainID)
	hash := signer.Hash(tx)
	sig, err := crypto.Sign(hash[:], key.PrivateKey)
	if err != nil {
		return nil, err
	}
	return tx.WithSignature(signer, sig)
}

// GetKey returns the private key for an unlocked account
func (ks *KeyStore) GetKey(addr common.Address) (*ecdsa.PrivateKey, error) {
	ks.mu.RLock()
	defer ks.mu.RUnlock()

	key, err := ks.getUnlockedKey(addr)
	if err != nil {
		return nil, err
	}

	return key.PrivateKey, nil
}

// Addresses returns the addresses of all accounts in the keystore
func (ks *KeyStore) Addresses() []common.Address {
	ks.mu.RLock()
	defer ks.mu.RUnlock()

	addresses := make([]common.Address, 0, len(ks.accounts))
	for addr := range ks.accounts {
		addresses = append(addresses, addr)
	}
	return addresses
}

// marshal converts encryptedKeyJSON to bytes
func (ek *encryptedKeyJSON) marshal() ([]byte, error) {
	return json.Marshal(ek)
}
