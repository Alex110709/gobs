// Copyright 2024 The Obsidian Authors
// This file is part of the Obsidian library.

package rpc

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	obstypes "github.com/obsidian-chain/obsidian/core/types"
)

var (
	ErrAccountNotFound = errors.New("account not found")
	ErrAccountLocked   = errors.New("account is locked")
	ErrPassphrase      = errors.New("invalid passphrase")
)

// KeystoreBackend defines the keystore operations needed by the Personal API
type KeystoreBackend interface {
	// Account management
	Accounts() []common.Address
	HasAddress(addr common.Address) bool
	NewAccount(password string) (common.Address, error)
	ImportRawKey(privateKey *ecdsa.PrivateKey, password string) (common.Address, error)
	ImportHex(hexKey string, password string) (common.Address, error)
	DeleteAccount(addr common.Address, password string) error

	// Unlock/Lock
	Unlock(addr common.Address, password string, duration time.Duration) error
	Lock(addr common.Address) error
	IsUnlocked(addr common.Address) bool

	// Signing
	SignHash(addr common.Address, hash []byte) ([]byte, error)
	SignTx(addr common.Address, tx *obstypes.StealthTransaction, chainID *big.Int) (*obstypes.StealthTransaction, error)
	SignTxWithPassword(addr common.Address, password string, tx *obstypes.StealthTransaction, chainID *big.Int) (*obstypes.StealthTransaction, error)
}

// PersonalBackend combines the regular backend with keystore functionality
type PersonalBackend interface {
	Backend
	GetKeystore() interface{} // Returns KeystoreBackend, typed as interface{} to avoid import cycles
}

// PrivateAccountAPI provides personal account management
type PrivateAccountAPI struct {
	backend PersonalBackend
}

// NewPrivateAccountAPI creates a new personal account API
func NewPrivateAccountAPI(b PersonalBackend) *PrivateAccountAPI {
	return &PrivateAccountAPI{backend: b}
}

// getKeystore returns the keystore backend with proper type assertion
func (api *PrivateAccountAPI) getKeystore() KeystoreBackend {
	ks := api.backend.GetKeystore()
	if ks == nil {
		return nil
	}
	if kb, ok := ks.(KeystoreBackend); ok {
		return kb
	}
	return nil
}

// ListAccounts returns all accounts in the keystore
func (api *PrivateAccountAPI) ListAccounts() []common.Address {
	ks := api.getKeystore()
	if ks == nil {
		return []common.Address{}
	}
	return ks.Accounts()
}

// NewAccount creates a new account with the given password
func (api *PrivateAccountAPI) NewAccount(password string) (common.Address, error) {
	ks := api.getKeystore()
	if ks == nil {
		return common.Address{}, errors.New("keystore not available")
	}
	return ks.NewAccount(password)
}

// ImportRawKey imports a hex-encoded private key
func (api *PrivateAccountAPI) ImportRawKey(hexKey string, password string) (common.Address, error) {
	ks := api.getKeystore()
	if ks == nil {
		return common.Address{}, errors.New("keystore not available")
	}
	return ks.ImportHex(hexKey, password)
}

// UnlockAccount unlocks an account for a duration (in seconds)
// If duration is 0, it defaults to 300 seconds (5 minutes)
func (api *PrivateAccountAPI) UnlockAccount(ctx context.Context, addr common.Address, password string, duration *uint64) (bool, error) {
	ks := api.getKeystore()
	if ks == nil {
		return false, errors.New("keystore not available")
	}

	const defaultDuration = 300 * time.Second
	var d time.Duration
	if duration == nil || *duration == 0 {
		d = defaultDuration
	} else {
		d = time.Duration(*duration) * time.Second
	}

	if err := ks.Unlock(addr, password, d); err != nil {
		return false, err
	}
	return true, nil
}

// LockAccount locks an account
func (api *PrivateAccountAPI) LockAccount(addr common.Address) bool {
	ks := api.getKeystore()
	if ks == nil {
		return false
	}
	return ks.Lock(addr) == nil
}

// SendTransaction creates, signs, and sends a transaction
func (api *PrivateAccountAPI) SendTransaction(ctx context.Context, args SendTxArgs, password string) (common.Hash, error) {
	ks := api.getKeystore()
	if ks == nil {
		return common.Hash{}, errors.New("keystore not available")
	}

	// Build the transaction
	tx, err := api.buildTransaction(ctx, args)
	if err != nil {
		return common.Hash{}, err
	}

	// Sign with password
	signed, err := ks.SignTxWithPassword(args.From, password, tx, api.backend.ChainID())
	if err != nil {
		return common.Hash{}, err
	}

	// Send the signed transaction
	return api.backend.SendTransaction(ctx, signed)
}

// SignTransaction signs a transaction without sending it
func (api *PrivateAccountAPI) SignTransaction(ctx context.Context, args SendTxArgs, password string) (*SignedTx, error) {
	ks := api.getKeystore()
	if ks == nil {
		return nil, errors.New("keystore not available")
	}

	// Build the transaction
	tx, err := api.buildTransaction(ctx, args)
	if err != nil {
		return nil, err
	}

	// Sign with password
	signed, err := ks.SignTxWithPassword(args.From, password, tx, api.backend.ChainID())
	if err != nil {
		return nil, err
	}

	// Return the signed transaction
	return &SignedTx{
		Raw: signed,
		Tx:  RPCMarshalTransaction(signed, common.Hash{}, 0, 0),
	}, nil
}

// Sign signs arbitrary data with an unlocked account
func (api *PrivateAccountAPI) Sign(ctx context.Context, data hexutil.Bytes, addr common.Address, password string) (hexutil.Bytes, error) {
	ks := api.getKeystore()
	if ks == nil {
		return nil, errors.New("keystore not available")
	}

	// Hash the data with Ethereum signed message prefix
	msg := fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(data), data)
	hash := crypto.Keccak256([]byte(msg))

	// Sign the hash
	// First try with unlocked account
	if ks.IsUnlocked(addr) {
		sig, err := ks.SignHash(addr, hash)
		if err != nil {
			return nil, err
		}
		return sig, nil
	}

	// If not unlocked, we need to unlock temporarily with password
	if err := ks.Unlock(addr, password, time.Second); err != nil {
		return nil, err
	}
	defer func() { _ = ks.Lock(addr) }()

	sig, err := ks.SignHash(addr, hash)
	if err != nil {
		return nil, err
	}
	return sig, nil
}

// EcRecover recovers the address from a signature
func (api *PrivateAccountAPI) EcRecover(ctx context.Context, data hexutil.Bytes, sig hexutil.Bytes) (common.Address, error) {
	if len(sig) != crypto.SignatureLength {
		return common.Address{}, errors.New("invalid signature length")
	}

	// Hash the data with Ethereum signed message prefix
	msg := fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(data), data)
	hash := crypto.Keccak256([]byte(msg))

	// Recover the public key
	sigCopy := make([]byte, len(sig))
	copy(sigCopy, sig)
	if sigCopy[crypto.RecoveryIDOffset] >= 27 {
		sigCopy[crypto.RecoveryIDOffset] -= 27
	}

	pubkey, err := crypto.SigToPub(hash, sigCopy)
	if err != nil {
		return common.Address{}, err
	}

	return crypto.PubkeyToAddress(*pubkey), nil
}

// buildTransaction builds a transaction from the given args
func (api *PrivateAccountAPI) buildTransaction(ctx context.Context, args SendTxArgs) (*obstypes.StealthTransaction, error) {
	// Get nonce if not specified
	var nonce uint64
	if args.Nonce != nil {
		nonce = uint64(*args.Nonce)
	} else {
		var err error
		nonce, err = api.backend.GetNonce(ctx, args.From, -1)
		if err != nil {
			return nil, err
		}
	}

	// Get gas price if not specified
	var gasPrice *big.Int
	if args.GasPrice != nil {
		gasPrice = args.GasPrice.ToInt()
	} else {
		var err error
		gasPrice, err = api.backend.SuggestGasPrice(ctx)
		if err != nil {
			return nil, err
		}
	}

	// Default gas limit
	var gas uint64 = 21000
	if args.Gas != nil {
		gas = uint64(*args.Gas)
	}

	// Get value
	var value *big.Int = big.NewInt(0)
	if args.Value != nil {
		value = args.Value.ToInt()
	}

	// Get data
	var data []byte
	if args.Data != nil {
		data = *args.Data
	}
	if args.Input != nil {
		data = *args.Input
	}

	// Create the transaction
	if args.To != nil {
		// Regular transaction
		return obstypes.NewStealthTransaction(nonce, *args.To, value, gas, gasPrice, data, nil, 0), nil
	}
	// Contract creation
	return obstypes.NewStealthContractCreation(nonce, value, gas, gasPrice, data, nil, 0), nil
}

// SendTxArgs represents the arguments to submit a transaction
type SendTxArgs struct {
	From     common.Address  `json:"from"`
	To       *common.Address `json:"to"`
	Gas      *hexutil.Uint64 `json:"gas"`
	GasPrice *hexutil.Big    `json:"gasPrice"`
	Value    *hexutil.Big    `json:"value"`
	Nonce    *hexutil.Uint64 `json:"nonce"`
	Data     *hexutil.Bytes  `json:"data"`
	Input    *hexutil.Bytes  `json:"input"`
}

// SignedTx represents a signed transaction result
type SignedTx struct {
	Raw *obstypes.StealthTransaction `json:"raw"`
	Tx  map[string]interface{}       `json:"tx"`
}
