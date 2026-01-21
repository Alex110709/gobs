// Copyright 2024 The Obsidian Authors
// This file is part of the Obsidian library.

package keystore

import (
	"crypto/ecdsa"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	obstypes "github.com/obsidian-chain/obsidian/core/types"
)

// KeystoreWrapper wraps a KeyStore and implements the KeystoreBackend interface
// used by the RPC API for account management
type KeystoreWrapper struct {
	ks *KeyStore
}

// NewKeystoreWrapper creates a new keystore wrapper
func NewKeystoreWrapper(ks *KeyStore) *KeystoreWrapper {
	return &KeystoreWrapper{ks: ks}
}

// Accounts returns the addresses of all accounts in the keystore
func (w *KeystoreWrapper) Accounts() []common.Address {
	return w.ks.Addresses()
}

// HasAddress checks if an address is in the keystore
func (w *KeystoreWrapper) HasAddress(addr common.Address) bool {
	return w.ks.HasAddress(addr)
}

// NewAccount creates a new account with the given password
func (w *KeystoreWrapper) NewAccount(password string) (common.Address, error) {
	account, err := w.ks.NewAccount(password)
	if err != nil {
		return common.Address{}, err
	}
	return account.Address, nil
}

// ImportRawKey imports a private key
func (w *KeystoreWrapper) ImportRawKey(privateKey *ecdsa.PrivateKey, password string) (common.Address, error) {
	account, err := w.ks.Import(privateKey, password)
	if err != nil {
		return common.Address{}, err
	}
	return account.Address, nil
}

// ImportHex imports a hex-encoded private key
func (w *KeystoreWrapper) ImportHex(hexKey string, password string) (common.Address, error) {
	account, err := w.ks.ImportHex(hexKey, password)
	if err != nil {
		return common.Address{}, err
	}
	return account.Address, nil
}

// DeleteAccount deletes an account from the keystore
func (w *KeystoreWrapper) DeleteAccount(addr common.Address, password string) error {
	return w.ks.Delete(addr, password)
}

// Unlock unlocks an account for a specified duration
// If duration is 0, the account remains unlocked indefinitely
func (w *KeystoreWrapper) Unlock(addr common.Address, password string, duration time.Duration) error {
	return w.ks.TimedUnlock(addr, password, duration)
}

// Lock locks an account
func (w *KeystoreWrapper) Lock(addr common.Address) error {
	return w.ks.Lock(addr)
}

// IsUnlocked checks if an account is unlocked
func (w *KeystoreWrapper) IsUnlocked(addr common.Address) bool {
	return w.ks.IsUnlocked(addr)
}

// SignHash signs a hash with an unlocked account
func (w *KeystoreWrapper) SignHash(addr common.Address, hash []byte) ([]byte, error) {
	return w.ks.SignHash(addr, hash)
}

// SignTx signs a transaction with an unlocked account
func (w *KeystoreWrapper) SignTx(addr common.Address, tx *obstypes.StealthTransaction, chainID *big.Int) (*obstypes.StealthTransaction, error) {
	return w.ks.SignTx(addr, tx, chainID)
}

// SignTxWithPassword signs a transaction without unlocking the account
func (w *KeystoreWrapper) SignTxWithPassword(addr common.Address, password string, tx *obstypes.StealthTransaction, chainID *big.Int) (*obstypes.StealthTransaction, error) {
	return w.ks.SignTxWithPassword(addr, password, tx, chainID)
}
