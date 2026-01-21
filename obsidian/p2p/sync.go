// Copyright 2024 The Obsidian Authors
// This file is part of the Obsidian library.

package p2p

import (
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p"
	obstypes "github.com/obsidian-chain/obsidian/core/types"
)

// Synchronizer handles blockchain synchronization with proper request/response tracking
type Synchronizer struct {
	backend Backend
	handler *Handler

	// State
	syncing      int32
	currentPeer  *Peer
	startBlock   uint64
	targetBlock  uint64
	currentBlock uint64

	// Pending blocks received during sync
	pendingBlocks map[uint64]*obstypes.ObsidianBlock
	pendingMu     sync.Mutex

	// Control
	doneCh   chan struct{}
	cancelCh chan struct{}

	log log.Logger
}

// NewSynchronizer creates a new blockchain synchronizer
func NewSynchronizer(backend Backend, handler *Handler) *Synchronizer {
	return &Synchronizer{
		backend:       backend,
		handler:       handler,
		pendingBlocks: make(map[uint64]*obstypes.ObsidianBlock),
		log:           log.New("module", "sync"),
	}
}

// Start initiates the sync process with the best available peer
func (s *Synchronizer) Start() error {
	if !atomic.CompareAndSwapInt32(&s.syncing, 0, 1) {
		return ErrAlreadySyncing
	}

	s.doneCh = make(chan struct{})
	s.cancelCh = make(chan struct{})

	go s.run()
	return nil
}

// Stop stops the synchronizer
func (s *Synchronizer) Stop() {
	if s.cancelCh != nil {
		select {
		case <-s.cancelCh:
		default:
			close(s.cancelCh)
		}
	}
}

// Syncing returns whether sync is in progress
func (s *Synchronizer) Syncing() bool {
	return atomic.LoadInt32(&s.syncing) == 1
}

// Progress returns current sync progress
func (s *Synchronizer) Progress() (start, current, target uint64, syncing bool) {
	return s.startBlock, atomic.LoadUint64(&s.currentBlock), s.targetBlock, s.Syncing()
}

// run is the main sync loop
func (s *Synchronizer) run() {
	defer func() {
		atomic.StoreInt32(&s.syncing, 0)
		close(s.doneCh)
	}()

	// Wait a bit for peers to connect
	time.Sleep(2 * time.Second)

	for {
		select {
		case <-s.cancelCh:
			return
		default:
		}

		// Find best peer
		peer := s.findBestPeer()
		if peer == nil {
			s.log.Debug("No peers available for sync")
			time.Sleep(5 * time.Second)
			continue
		}

		// Check if peer is ahead
		head := s.backend.CurrentBlock()
		ourTD := s.backend.GetTD(head.Hash())
		if ourTD == nil {
			ourTD = big.NewInt(0)
		}

		peer.lock.RLock()
		peerTD := peer.td
		peerNum := peer.number
		peer.lock.RUnlock()

		if peerTD == nil || peerTD.Cmp(ourTD) <= 0 {
			s.log.Debug("Already synced or peer not ahead",
				"ourHead", head.Number.Uint64(),
				"ourTD", ourTD,
				"peerHead", peerNum,
				"peerTD", peerTD,
			)
			time.Sleep(10 * time.Second)
			continue
		}

		// Perform sync
		if err := s.syncWithPeer(peer, peerNum); err != nil {
			s.log.Error("Sync failed", "peer", peer.id[:16], "err", err)
			time.Sleep(5 * time.Second)
		}
	}
}

// findBestPeer finds the peer with highest TD
func (s *Synchronizer) findBestPeer() *Peer {
	if s.handler == nil {
		return nil
	}

	peers := s.handler.Peers()
	if len(peers) == 0 {
		return nil
	}

	var best *Peer
	var bestTD *big.Int

	for _, p := range peers {
		p.lock.RLock()
		td := p.td
		p.lock.RUnlock()

		if td != nil && (bestTD == nil || td.Cmp(bestTD) > 0) {
			best = p
			bestTD = td
		}
	}

	return best
}

// syncWithPeer synchronizes blockchain with a specific peer
func (s *Synchronizer) syncWithPeer(peer *Peer, targetNum uint64) error {
	head := s.backend.CurrentBlock()
	ourNum := head.Number.Uint64()

	if targetNum <= ourNum {
		return nil
	}

	s.currentPeer = peer
	s.startBlock = ourNum
	s.targetBlock = targetNum
	atomic.StoreUint64(&s.currentBlock, ourNum)

	s.log.Info("Starting sync",
		"peer", peer.id[:16],
		"from", ourNum+1,
		"to", targetNum,
	)

	startTime := time.Now()
	downloaded := uint64(0)

	// Download and insert blocks one by one
	// This is simpler and more reliable than batch processing
	for num := ourNum + 1; num <= targetNum; {
		select {
		case <-s.cancelCh:
			return ErrCancelled
		default:
		}

		// Check if we already have this block pending
		s.pendingMu.Lock()
		block, exists := s.pendingBlocks[num]
		if exists {
			delete(s.pendingBlocks, num)
		}
		s.pendingMu.Unlock()

		if !exists {
			// Request the block
			if err := p2p.Send(peer.rw, GetBlockByNumberMsg, num); err != nil {
				return fmt.Errorf("failed to request block %d: %v", num, err)
			}

			// Wait for block with timeout
			block = s.waitForBlock(num, 10*time.Second)
			if block == nil {
				s.log.Warn("Timeout waiting for block", "number", num)
				// Try again
				continue
			}
		}

		// Validate block number
		if block.NumberU64() != num {
			s.log.Warn("Block number mismatch", "expected", num, "got", block.NumberU64())
			continue
		}

		// Insert block
		if err := s.backend.InsertBlock(block); err != nil {
			s.log.Error("Failed to insert block",
				"number", num,
				"hash", block.Hash().Hex()[:16],
				"err", err,
			)
			// Skip this block and continue
			num++
			continue
		}

		downloaded++
		atomic.StoreUint64(&s.currentBlock, num)
		num++

		// Log progress periodically
		if downloaded%100 == 0 || num > targetNum {
			elapsed := time.Since(startTime)
			bps := float64(downloaded) / elapsed.Seconds()
			remaining := targetNum - num + 1
			eta := time.Duration(float64(remaining)/bps) * time.Second

			s.log.Info("Sync progress",
				"current", num-1,
				"target", targetNum,
				"downloaded", downloaded,
				"bps", fmt.Sprintf("%.2f", bps),
				"eta", eta.Round(time.Second),
			)
		}
	}

	elapsed := time.Since(startTime)
	s.log.Info("Sync completed",
		"blocks", downloaded,
		"duration", elapsed.Round(time.Millisecond),
		"blocks/sec", fmt.Sprintf("%.2f", float64(downloaded)/elapsed.Seconds()),
	)

	s.currentPeer = nil
	return nil
}

// waitForBlock waits for a specific block number to arrive
func (s *Synchronizer) waitForBlock(num uint64, timeout time.Duration) *obstypes.ObsidianBlock {
	deadline := time.Now().Add(timeout)
	checkInterval := 100 * time.Millisecond

	for time.Now().Before(deadline) {
		s.pendingMu.Lock()
		block, exists := s.pendingBlocks[num]
		if exists {
			delete(s.pendingBlocks, num)
			s.pendingMu.Unlock()
			return block
		}
		s.pendingMu.Unlock()

		select {
		case <-s.cancelCh:
			return nil
		case <-time.After(checkInterval):
		}
	}

	return nil
}

// DeliverBlock is called when a block is received during sync
func (s *Synchronizer) DeliverBlock(block *obstypes.ObsidianBlock) {
	if !s.Syncing() {
		return
	}

	s.pendingMu.Lock()
	s.pendingBlocks[block.NumberU64()] = block
	s.pendingMu.Unlock()
}
