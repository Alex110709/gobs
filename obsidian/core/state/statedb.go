// Copyright 2024 The Obsidian Authors
// This file is part of the Obsidian library.

package state

import (
	"bytes"
	"errors"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"

	"github.com/obsidian-chain/obsidian/core/rawdb"
)

var (
	// emptyCodeHash is the hash of empty code
	emptyCodeHash = crypto.Keccak256Hash(nil)
	// emptyRoot is the hash of empty trie
	emptyRoot = common.HexToHash("56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421")

	ErrStateNotFound = errors.New("state not found")
)

// StateDB is the database for storing account state
type StateDB struct {
	db      *rawdb.Database
	root    common.Hash
	objects map[common.Address]*stateObject
	dirty   map[common.Address]struct{}
	deleted map[common.Address]struct{}
	lock    sync.RWMutex

	// Journal for reverts
	journal        *journal
	validRevisions []revision
	nextRevisionId int

	// Transaction context
	txHash  common.Hash
	txIndex int

	// Logs
	logs    map[common.Hash][]*Log
	logSize uint
}

// stateObject represents an account
type stateObject struct {
	address  common.Address
	addrHash common.Hash
	data     Account
	code     []byte
	codeHash []byte
	dirty    bool
	suicided bool
	deleted  bool

	// Storage changes
	originStorage  map[common.Hash]common.Hash
	pendingStorage map[common.Hash]common.Hash
	dirtyStorage   map[common.Hash]common.Hash
}

// Account represents an Obsidian account
type Account struct {
	Nonce    uint64
	Balance  *big.Int
	Root     common.Hash // storage root
	CodeHash []byte
}

// journal records state changes for reversion
type journal struct {
	entries []journalEntry
}

type journalEntry interface {
	revert(*StateDB)
}

type revision struct {
	id           int
	journalIndex int
}

// New creates a new state database with persistence
func New(root common.Hash, db *rawdb.Database) (*StateDB, error) {
	return NewWithDB(root, db)
}

// NewWithDB creates a new state database backed by LevelDB
func NewWithDB(root common.Hash, db *rawdb.Database) (*StateDB, error) {
	sdb := &StateDB{
		db:      db,
		root:    root,
		objects: make(map[common.Address]*stateObject),
		dirty:   make(map[common.Address]struct{}),
		deleted: make(map[common.Address]struct{}),
		journal: &journal{},
		logs:    make(map[common.Hash][]*Log),
	}

	// If root is not empty, try to load from database
	if root != (common.Hash{}) && root != emptyRoot {
		// Load state from database
		if err := sdb.loadFromDB(root); err != nil {
			// State not found, start fresh
			sdb.root = emptyRoot
		}
	}

	return sdb, nil
}

// loadFromDB loads state from the database
func (s *StateDB) loadFromDB(root common.Hash) error {
	// This is a simplified implementation
	// In production, you'd load the merkle patricia trie
	return nil
}

// NewMemoryStateDB creates an in-memory state database
func NewMemoryStateDB() *StateDB {
	return &StateDB{
		root:    emptyRoot,
		objects: make(map[common.Address]*stateObject),
		dirty:   make(map[common.Address]struct{}),
		deleted: make(map[common.Address]struct{}),
		journal: &journal{},
		logs:    make(map[common.Hash][]*Log),
	}
}

// Database returns the underlying database
func (s *StateDB) Database() *rawdb.Database {
	return s.db
}

// SetTxContext sets the current transaction context
func (s *StateDB) SetTxContext(txHash common.Hash, txIndex int) {
	s.txHash = txHash
	s.txIndex = txIndex
}

// GetOrNewStateObject returns the state object for an address, creating one if needed
func (s *StateDB) GetOrNewStateObject(addr common.Address) *stateObject {
	s.lock.Lock()
	defer s.lock.Unlock()

	obj := s.objects[addr]
	if obj == nil || obj.deleted {
		obj = s.createObject(addr)
	}
	return obj
}

// createObject creates a new state object
func (s *StateDB) createObject(addr common.Address) *stateObject {
	obj := &stateObject{
		address:  addr,
		addrHash: crypto.Keccak256Hash(addr[:]),
		data: Account{
			Balance:  big.NewInt(0),
			CodeHash: emptyCodeHash.Bytes(),
			Root:     emptyRoot,
		},
		originStorage:  make(map[common.Hash]common.Hash),
		pendingStorage: make(map[common.Hash]common.Hash),
		dirtyStorage:   make(map[common.Hash]common.Hash),
	}
	s.objects[addr] = obj
	return obj
}

// getStateObject returns the state object for an address
func (s *StateDB) getStateObject(addr common.Address) *stateObject {
	s.lock.RLock()
	obj := s.objects[addr]
	s.lock.RUnlock()

	if obj != nil && !obj.deleted {
		return obj
	}

	// Try to load from database
	if s.db != nil {
		return s.loadStateObject(addr)
	}

	return nil
}

// loadStateObject loads a state object from the database
func (s *StateDB) loadStateObject(addr common.Address) *stateObject {
	addrHash := crypto.Keccak256Hash(addr[:])
	data := rawdb.ReadAccountData(s.db, addrHash)
	if data == nil {
		return nil
	}

	var account Account
	if err := rlp.DecodeBytes(data, &account); err != nil {
		return nil
	}

	obj := &stateObject{
		address:        addr,
		addrHash:       addrHash,
		data:           account,
		originStorage:  make(map[common.Hash]common.Hash),
		pendingStorage: make(map[common.Hash]common.Hash),
		dirtyStorage:   make(map[common.Hash]common.Hash),
	}

	// Load code if exists
	if len(account.CodeHash) > 0 && !bytes.Equal(account.CodeHash, emptyCodeHash.Bytes()) {
		obj.code = rawdb.ReadCode(s.db, common.BytesToHash(account.CodeHash))
		obj.codeHash = account.CodeHash
	}

	s.lock.Lock()
	s.objects[addr] = obj
	s.lock.Unlock()

	return obj
}

// GetBalance returns the balance of an account
func (s *StateDB) GetBalance(addr common.Address) *big.Int {
	obj := s.getStateObject(addr)
	if obj != nil {
		return new(big.Int).Set(obj.data.Balance)
	}
	return big.NewInt(0)
}

// SetBalance sets the balance of an account
func (s *StateDB) SetBalance(addr common.Address, amount *big.Int) {
	obj := s.GetOrNewStateObject(addr)
	obj.data.Balance = new(big.Int).Set(amount)
	obj.dirty = true
	s.dirty[addr] = struct{}{}
}

// AddBalance adds amount to an account's balance
func (s *StateDB) AddBalance(addr common.Address, amount *big.Int) {
	obj := s.GetOrNewStateObject(addr)
	obj.data.Balance = new(big.Int).Add(obj.data.Balance, amount)
	obj.dirty = true
	s.dirty[addr] = struct{}{}
}

// SubBalance subtracts amount from an account's balance
func (s *StateDB) SubBalance(addr common.Address, amount *big.Int) {
	obj := s.GetOrNewStateObject(addr)
	obj.data.Balance = new(big.Int).Sub(obj.data.Balance, amount)
	obj.dirty = true
	s.dirty[addr] = struct{}{}
}

// GetNonce returns the nonce of an account
func (s *StateDB) GetNonce(addr common.Address) uint64 {
	obj := s.getStateObject(addr)
	if obj != nil {
		return obj.data.Nonce
	}
	return 0
}

// SetNonce sets the nonce of an account
func (s *StateDB) SetNonce(addr common.Address, nonce uint64) {
	obj := s.GetOrNewStateObject(addr)
	obj.data.Nonce = nonce
	obj.dirty = true
	s.dirty[addr] = struct{}{}
}

// GetCode returns the code of an account
func (s *StateDB) GetCode(addr common.Address) []byte {
	obj := s.getStateObject(addr)
	if obj != nil {
		return obj.code
	}
	return nil
}

// SetCode sets the code of an account
func (s *StateDB) SetCode(addr common.Address, code []byte) {
	obj := s.GetOrNewStateObject(addr)
	obj.code = code
	obj.codeHash = crypto.Keccak256(code)
	obj.data.CodeHash = obj.codeHash
	obj.dirty = true
	s.dirty[addr] = struct{}{}
}

// GetCodeHash returns the code hash of an account
func (s *StateDB) GetCodeHash(addr common.Address) common.Hash {
	obj := s.getStateObject(addr)
	if obj != nil && len(obj.codeHash) > 0 {
		return common.BytesToHash(obj.codeHash)
	}
	return common.Hash{}
}

// GetCodeSize returns the code size of an account
func (s *StateDB) GetCodeSize(addr common.Address) int {
	obj := s.getStateObject(addr)
	if obj != nil {
		return len(obj.code)
	}
	return 0
}

// GetState returns the value of a storage key
func (s *StateDB) GetState(addr common.Address, key common.Hash) common.Hash {
	obj := s.getStateObject(addr)
	if obj != nil {
		// Check dirty storage
		if val, ok := obj.dirtyStorage[key]; ok {
			return val
		}
		// Check pending storage
		if val, ok := obj.pendingStorage[key]; ok {
			return val
		}
		// Check origin storage
		if val, ok := obj.originStorage[key]; ok {
			return val
		}
		// Load from database
		if s.db != nil {
			keyHash := crypto.Keccak256Hash(key[:])
			data := rawdb.ReadStorageData(s.db, obj.addrHash, keyHash)
			if data != nil {
				val := common.BytesToHash(data)
				obj.originStorage[key] = val
				return val
			}
		}
	}
	return common.Hash{}
}

// SetState sets the value of a storage key
func (s *StateDB) SetState(addr common.Address, key, value common.Hash) {
	obj := s.GetOrNewStateObject(addr)
	obj.dirtyStorage[key] = value
	obj.dirty = true
	s.dirty[addr] = struct{}{}
}

// Exist returns whether an account exists
func (s *StateDB) Exist(addr common.Address) bool {
	return s.getStateObject(addr) != nil
}

// Empty returns whether an account is empty
func (s *StateDB) Empty(addr common.Address) bool {
	obj := s.getStateObject(addr)
	if obj == nil {
		return true
	}
	return obj.data.Nonce == 0 && obj.data.Balance.Sign() == 0 && len(obj.code) == 0
}

// CreateAccount creates a new account
func (s *StateDB) CreateAccount(addr common.Address) {
	s.GetOrNewStateObject(addr)
}

// Suicide marks an account for deletion
func (s *StateDB) Suicide(addr common.Address) bool {
	obj := s.getStateObject(addr)
	if obj == nil {
		return false
	}
	obj.suicided = true
	obj.data.Balance = big.NewInt(0)
	s.dirty[addr] = struct{}{}
	return true
}

// HasSuicided returns whether an account has been suicided
func (s *StateDB) HasSuicided(addr common.Address) bool {
	obj := s.getStateObject(addr)
	if obj != nil {
		return obj.suicided
	}
	return false
}

// AddLog adds a log entry
func (s *StateDB) AddLog(log *Log) {
	log.TxHash = s.txHash
	log.TxIndex = uint(s.txIndex)
	log.Index = s.logSize
	s.logs[s.txHash] = append(s.logs[s.txHash], log)
	s.logSize++
}

// GetLogs returns all logs for a transaction
func (s *StateDB) GetLogs(txHash common.Hash, blockNumber uint64, blockHash common.Hash) []*Log {
	logs := s.logs[txHash]
	for _, l := range logs {
		l.BlockNumber = blockNumber
		l.BlockHash = blockHash
	}
	return logs
}

// Logs returns all logs
func (s *StateDB) Logs() []*Log {
	var logs []*Log
	for _, lgs := range s.logs {
		logs = append(logs, lgs...)
	}
	return logs
}

// Snapshot creates a snapshot for later reversion
func (s *StateDB) Snapshot() int {
	id := s.nextRevisionId
	s.nextRevisionId++
	s.validRevisions = append(s.validRevisions, revision{id: id, journalIndex: len(s.journal.entries)})
	return id
}

// RevertToSnapshot reverts to a previous snapshot
func (s *StateDB) RevertToSnapshot(revid int) {
	// Find the snapshot index
	idx := -1
	for i := len(s.validRevisions) - 1; i >= 0; i-- {
		if s.validRevisions[i].id == revid {
			idx = i
			break
		}
	}
	if idx < 0 {
		return
	}

	// Revert journal entries
	snapshot := s.validRevisions[idx]
	for i := len(s.journal.entries) - 1; i >= snapshot.journalIndex; i-- {
		s.journal.entries[i].revert(s)
	}
	s.journal.entries = s.journal.entries[:snapshot.journalIndex]
	s.validRevisions = s.validRevisions[:idx]
}

// Finalise finalizes the state
func (s *StateDB) Finalise(deleteEmptyObjects bool) {
	s.lock.Lock()
	defer s.lock.Unlock()

	for addr := range s.dirty {
		obj := s.objects[addr]
		if obj == nil {
			continue
		}

		// Handle suicided accounts
		if obj.suicided {
			obj.deleted = true
			s.deleted[addr] = struct{}{}
			continue
		}

		// Move dirty storage to pending
		for key, value := range obj.dirtyStorage {
			obj.pendingStorage[key] = value
		}
		obj.dirtyStorage = make(map[common.Hash]common.Hash)

		if deleteEmptyObjects && s.emptyObject(obj) {
			obj.deleted = true
			s.deleted[addr] = struct{}{}
		}
	}
	s.dirty = make(map[common.Address]struct{})
}

func (s *StateDB) emptyObject(obj *stateObject) bool {
	return obj.data.Nonce == 0 && obj.data.Balance.Sign() == 0 && len(obj.code) == 0
}

// IntermediateRoot computes the state root
func (s *StateDB) IntermediateRoot(deleteEmptyObjects bool) common.Hash {
	s.Finalise(deleteEmptyObjects)

	// Compute a simple hash of all accounts
	// In production, this would be a proper Merkle Patricia Trie root
	hasher := crypto.NewKeccakState()

	s.lock.RLock()
	defer s.lock.RUnlock()

	for addr, obj := range s.objects {
		if obj.deleted {
			continue
		}
		hasher.Write(addr[:])
		data, _ := rlp.EncodeToBytes(&obj.data)
		hasher.Write(data)
	}

	var root common.Hash
	hasher.Read(root[:])

	if root == (common.Hash{}) {
		return emptyRoot
	}

	s.root = root
	return root
}

// Commit commits the state changes
func (s *StateDB) Commit(deleteEmptyObjects bool) (common.Hash, error) {
	root := s.IntermediateRoot(deleteEmptyObjects)
	return root, nil
}

// CommitToDB persists state changes to the database
func (s *StateDB) CommitToDB(db *rawdb.Database, root common.Hash) error {
	if db == nil {
		return nil
	}

	s.lock.RLock()
	defer s.lock.RUnlock()

	batch := db.NewBatch()

	// Write accounts
	for _, obj := range s.objects {
		if obj.deleted {
			// Delete account
			rawdb.DeleteAccountData(db, obj.addrHash)
			continue
		}

		// Encode and write account
		data, err := rlp.EncodeToBytes(&obj.data)
		if err != nil {
			return err
		}
		batch.Put(append([]byte("a"), obj.addrHash[:]...), data)

		// Write code if any
		if len(obj.code) > 0 && obj.dirty {
			codeHash := common.BytesToHash(obj.codeHash)
			rawdb.WriteCode(db, codeHash, obj.code)
		}

		// Write storage
		for key, value := range obj.pendingStorage {
			keyHash := crypto.Keccak256Hash(key[:])
			if value == (common.Hash{}) {
				rawdb.DeleteStorageData(db, obj.addrHash, keyHash)
			} else {
				rawdb.WriteStorageData(db, obj.addrHash, keyHash, value[:])
			}
		}
	}

	return batch.Write()
}

// Copy creates a deep copy of the state
func (s *StateDB) Copy() *StateDB {
	s.lock.RLock()
	defer s.lock.RUnlock()

	state := &StateDB{
		db:      s.db,
		root:    s.root,
		objects: make(map[common.Address]*stateObject),
		dirty:   make(map[common.Address]struct{}),
		deleted: make(map[common.Address]struct{}),
		journal: &journal{},
		logs:    make(map[common.Hash][]*Log),
	}

	for addr, obj := range s.objects {
		newObj := &stateObject{
			address:        obj.address,
			addrHash:       obj.addrHash,
			data:           obj.data,
			code:           obj.code,
			codeHash:       obj.codeHash,
			dirty:          obj.dirty,
			suicided:       obj.suicided,
			deleted:        obj.deleted,
			originStorage:  make(map[common.Hash]common.Hash),
			pendingStorage: make(map[common.Hash]common.Hash),
			dirtyStorage:   make(map[common.Hash]common.Hash),
		}
		newObj.data.Balance = new(big.Int).Set(obj.data.Balance)
		for k, v := range obj.originStorage {
			newObj.originStorage[k] = v
		}
		for k, v := range obj.pendingStorage {
			newObj.pendingStorage[k] = v
		}
		for k, v := range obj.dirtyStorage {
			newObj.dirtyStorage[k] = v
		}
		state.objects[addr] = newObj
	}

	for addr := range s.dirty {
		state.dirty[addr] = struct{}{}
	}

	for addr := range s.deleted {
		state.deleted[addr] = struct{}{}
	}

	// Copy logs
	for hash, logs := range s.logs {
		logsCopy := make([]*Log, len(logs))
		for i, log := range logs {
			logCopy := *log
			logsCopy[i] = &logCopy
		}
		state.logs[hash] = logsCopy
	}

	return state
}

// Log represents a log entry
type Log struct {
	Address     common.Address
	Topics      []common.Hash
	Data        []byte
	BlockNumber uint64
	TxHash      common.Hash
	TxIndex     uint
	BlockHash   common.Hash
	Index       uint
	Removed     bool
}

// StateDBInterface is the common interface for state database access
type StateDBInterface interface {
	GetBalance(addr common.Address) *big.Int
	GetNonce(addr common.Address) uint64
	AddBalance(addr common.Address, amount *big.Int)
	SubBalance(addr common.Address, amount *big.Int)
	SetNonce(addr common.Address, nonce uint64)
	GetCode(addr common.Address) []byte
	SetCode(addr common.Address, code []byte)
	CreateAccount(addr common.Address)
	Exist(addr common.Address) bool
	Finalise(deleteEmptyObjects bool)
	IntermediateRoot(deleteEmptyObjects bool) common.Hash
	Commit(deleteEmptyObjects bool) (common.Hash, error)
}
