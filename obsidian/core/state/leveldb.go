// Copyright 2024 The Obsidian Authors
// This file is part of the Obsidian library.

package state

import (
	"encoding/json"
	"fmt"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"
)

// Prefix keys for different data types
var (
	prefixBalance  = []byte("bal:")
	prefixNonce    = []byte("non:")
	prefixCode     = []byte("cod:")
	prefixCodeHash = []byte("coh:")
	prefixStorage  = []byte("sto:")
	prefixAccount  = []byte("acc:")
)

// LevelDBStateDB implements StateDB backed by LevelDB
type LevelDBStateDB struct {
	mu sync.RWMutex
	db *leveldb.DB

	// Cache for hot data
	balanceCache map[common.Address]*big.Int
	nonceCache   map[common.Address]uint64
	codeCache    map[common.Address][]byte
	storageCache map[common.Address]map[common.Hash]common.Hash

	// Dirty tracking
	dirtyBalances map[common.Address]bool
	dirtyNonces   map[common.Address]bool
	dirtyCodes    map[common.Address]bool
	dirtyStorage  map[common.Address]bool
}

// NewLevelDBStateDB creates a new LevelDB-backed state database
func NewLevelDBStateDB(path string) (*LevelDBStateDB, error) {
	opts := &opt.Options{
		BlockCacheCapacity:     32 * 1024 * 1024, // 32MB cache
		WriteBuffer:            16 * 1024 * 1024, // 16MB write buffer
		CompactionTableSize:    4 * 1024 * 1024,  // 4MB table size
		OpenFilesCacheCapacity: 500,
	}

	db, err := leveldb.OpenFile(path, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open leveldb: %w", err)
	}

	log.Info("LevelDB state database opened", "path", path)

	return &LevelDBStateDB{
		db:            db,
		balanceCache:  make(map[common.Address]*big.Int),
		nonceCache:    make(map[common.Address]uint64),
		codeCache:     make(map[common.Address][]byte),
		storageCache:  make(map[common.Address]map[common.Hash]common.Hash),
		dirtyBalances: make(map[common.Address]bool),
		dirtyNonces:   make(map[common.Address]bool),
		dirtyCodes:    make(map[common.Address]bool),
		dirtyStorage:  make(map[common.Address]bool),
	}, nil
}

// Close closes the database
func (s *LevelDBStateDB) Close() error {
	if err := s.Commit(); err != nil {
		log.Warn("Failed to commit on close", "err", err)
	}
	return s.db.Close()
}

// CreateAccount creates a new account
func (s *LevelDBStateDB) CreateAccount(addr common.Address) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Initialize with zero balance and nonce
	s.balanceCache[addr] = big.NewInt(0)
	s.nonceCache[addr] = 0
	s.dirtyBalances[addr] = true
	s.dirtyNonces[addr] = true
}

// GetBalance returns the balance of an account
func (s *LevelDBStateDB) GetBalance(addr common.Address) *big.Int {
	s.mu.RLock()
	if bal, ok := s.balanceCache[addr]; ok {
		s.mu.RUnlock()
		return new(big.Int).Set(bal)
	}
	s.mu.RUnlock()

	// Load from database
	s.mu.Lock()
	defer s.mu.Unlock()

	key := append(prefixBalance, addr.Bytes()...)
	data, err := s.db.Get(key, nil)
	if err != nil {
		s.balanceCache[addr] = big.NewInt(0)
		return big.NewInt(0)
	}

	bal := new(big.Int).SetBytes(data)
	s.balanceCache[addr] = bal
	return new(big.Int).Set(bal)
}

// SetBalance sets the balance of an account
func (s *LevelDBStateDB) SetBalance(addr common.Address, balance *big.Int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.balanceCache[addr] = new(big.Int).Set(balance)
	s.dirtyBalances[addr] = true
}

// AddBalance adds amount to account balance
func (s *LevelDBStateDB) AddBalance(addr common.Address, amount *big.Int) {
	current := s.GetBalance(addr)
	s.SetBalance(addr, new(big.Int).Add(current, amount))
}

// SubBalance subtracts amount from account balance
func (s *LevelDBStateDB) SubBalance(addr common.Address, amount *big.Int) {
	current := s.GetBalance(addr)
	s.SetBalance(addr, new(big.Int).Sub(current, amount))
}

// GetNonce returns the nonce of an account
func (s *LevelDBStateDB) GetNonce(addr common.Address) uint64 {
	s.mu.RLock()
	if nonce, ok := s.nonceCache[addr]; ok {
		s.mu.RUnlock()
		return nonce
	}
	s.mu.RUnlock()

	// Load from database
	s.mu.Lock()
	defer s.mu.Unlock()

	key := append(prefixNonce, addr.Bytes()...)
	data, err := s.db.Get(key, nil)
	if err != nil {
		s.nonceCache[addr] = 0
		return 0
	}

	var nonce uint64
	if len(data) >= 8 {
		for i := 0; i < 8; i++ {
			nonce |= uint64(data[i]) << (56 - i*8)
		}
	}

	s.nonceCache[addr] = nonce
	return nonce
}

// SetNonce sets the nonce of an account
func (s *LevelDBStateDB) SetNonce(addr common.Address, nonce uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.nonceCache[addr] = nonce
	s.dirtyNonces[addr] = true
}

// GetCode returns the code of an account
func (s *LevelDBStateDB) GetCode(addr common.Address) []byte {
	s.mu.RLock()
	if code, ok := s.codeCache[addr]; ok {
		s.mu.RUnlock()
		return code
	}
	s.mu.RUnlock()

	// Load from database
	s.mu.Lock()
	defer s.mu.Unlock()

	key := append(prefixCode, addr.Bytes()...)
	data, err := s.db.Get(key, nil)
	if err != nil {
		return nil
	}

	s.codeCache[addr] = data
	return data
}

// SetCode sets the code of an account
func (s *LevelDBStateDB) SetCode(addr common.Address, code []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.codeCache[addr] = code
	s.dirtyCodes[addr] = true
}

// GetCodeHash returns the code hash of an account
func (s *LevelDBStateDB) GetCodeHash(addr common.Address) common.Hash {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := append(prefixCodeHash, addr.Bytes()...)
	data, err := s.db.Get(key, nil)
	if err != nil {
		return common.Hash{}
	}

	return common.BytesToHash(data)
}

// GetState returns a storage value
func (s *LevelDBStateDB) GetState(addr common.Address, key common.Hash) common.Hash {
	s.mu.RLock()
	if storage, ok := s.storageCache[addr]; ok {
		if val, ok := storage[key]; ok {
			s.mu.RUnlock()
			return val
		}
	}
	s.mu.RUnlock()

	// Load from database
	s.mu.Lock()
	defer s.mu.Unlock()

	dbKey := append(prefixStorage, addr.Bytes()...)
	dbKey = append(dbKey, key.Bytes()...)

	data, err := s.db.Get(dbKey, nil)
	if err != nil {
		return common.Hash{}
	}

	val := common.BytesToHash(data)

	if s.storageCache[addr] == nil {
		s.storageCache[addr] = make(map[common.Hash]common.Hash)
	}
	s.storageCache[addr][key] = val

	return val
}

// SetState sets a storage value
func (s *LevelDBStateDB) SetState(addr common.Address, key, value common.Hash) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.storageCache[addr] == nil {
		s.storageCache[addr] = make(map[common.Hash]common.Hash)
	}
	s.storageCache[addr][key] = value
	s.dirtyStorage[addr] = true
}

// Commit persists all dirty data to the database
func (s *LevelDBStateDB) Commit() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	batch := new(leveldb.Batch)

	// Write dirty balances
	for addr := range s.dirtyBalances {
		if bal, ok := s.balanceCache[addr]; ok {
			key := append(prefixBalance, addr.Bytes()...)
			batch.Put(key, bal.Bytes())
		}
	}

	// Write dirty nonces
	for addr := range s.dirtyNonces {
		if nonce, ok := s.nonceCache[addr]; ok {
			key := append(prefixNonce, addr.Bytes()...)
			data := make([]byte, 8)
			for i := 0; i < 8; i++ {
				data[i] = byte(nonce >> (56 - i*8))
			}
			batch.Put(key, data)
		}
	}

	// Write dirty codes
	for addr := range s.dirtyCodes {
		if code, ok := s.codeCache[addr]; ok {
			key := append(prefixCode, addr.Bytes()...)
			batch.Put(key, code)
		}
	}

	// Write dirty storage
	for addr := range s.dirtyStorage {
		if storage, ok := s.storageCache[addr]; ok {
			for k, v := range storage {
				dbKey := append(prefixStorage, addr.Bytes()...)
				dbKey = append(dbKey, k.Bytes()...)
				batch.Put(dbKey, v.Bytes())
			}
		}
	}

	if err := s.db.Write(batch, nil); err != nil {
		return fmt.Errorf("failed to commit batch: %w", err)
	}

	// Clear dirty flags
	s.dirtyBalances = make(map[common.Address]bool)
	s.dirtyNonces = make(map[common.Address]bool)
	s.dirtyCodes = make(map[common.Address]bool)
	s.dirtyStorage = make(map[common.Address]bool)

	return nil
}

// Copy creates a copy of the state
func (s *LevelDBStateDB) Copy() StateDBInterface {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Create a memory-backed copy for temporary operations
	mem := NewMemoryStateDB()

	for addr, bal := range s.balanceCache {
		mem.SetBalance(addr, new(big.Int).Set(bal))
	}
	for addr, nonce := range s.nonceCache {
		mem.SetNonce(addr, nonce)
	}
	for addr, code := range s.codeCache {
		codeCopy := make([]byte, len(code))
		copy(codeCopy, code)
		mem.SetCode(addr, codeCopy)
	}
	for addr, storage := range s.storageCache {
		for k, v := range storage {
			mem.SetState(addr, k, v)
		}
	}

	return mem
}

// Exist checks if an account exists
func (s *LevelDBStateDB) Exist(addr common.Address) bool {
	s.mu.RLock()
	if _, ok := s.balanceCache[addr]; ok {
		s.mu.RUnlock()
		return true
	}
	s.mu.RUnlock()

	// Check database
	key := append(prefixAccount, addr.Bytes()...)
	exists, _ := s.db.Has(key, nil)
	return exists
}

// Empty checks if an account is empty
func (s *LevelDBStateDB) Empty(addr common.Address) bool {
	return s.GetBalance(addr).Sign() == 0 &&
		s.GetNonce(addr) == 0 &&
		len(s.GetCode(addr)) == 0
}

// GetRefund returns the current refund counter
func (s *LevelDBStateDB) GetRefund() uint64 {
	return 0 // Simplified implementation
}

// AddRefund adds gas to the refund counter
func (s *LevelDBStateDB) AddRefund(gas uint64) {
	// Simplified implementation
}

// SubRefund removes gas from the refund counter
func (s *LevelDBStateDB) SubRefund(gas uint64) {
	// Simplified implementation
}

// IntermediateRoot computes the current root hash
func (s *LevelDBStateDB) IntermediateRoot(deleteEmptyObjects bool) common.Hash {
	// Simplified implementation - returns a hash of all accounts
	return common.Hash{}
}

// Finalise finalises the state
func (s *LevelDBStateDB) Finalise(deleteEmptyObjects bool) {
	_ = s.Commit()
}

// GetAccountStats returns statistics about the database
func (s *LevelDBStateDB) GetAccountStats() (int, int64) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	count := 0
	var totalSize int64

	iter := s.db.NewIterator(util.BytesPrefix(prefixAccount), nil)
	defer iter.Release()

	for iter.Next() {
		count++
		totalSize += int64(len(iter.Value()))
	}

	return count, totalSize
}

// Export exports the entire state to JSON
func (s *LevelDBStateDB) Export() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	state := make(map[string]interface{})
	accounts := make(map[string]map[string]interface{})

	// Export from cache
	for addr, bal := range s.balanceCache {
		addrStr := addr.Hex()
		if accounts[addrStr] == nil {
			accounts[addrStr] = make(map[string]interface{})
		}
		accounts[addrStr]["balance"] = bal.String()
	}

	for addr, nonce := range s.nonceCache {
		addrStr := addr.Hex()
		if accounts[addrStr] == nil {
			accounts[addrStr] = make(map[string]interface{})
		}
		accounts[addrStr]["nonce"] = nonce
	}

	state["accounts"] = accounts
	return json.Marshal(state)
}
