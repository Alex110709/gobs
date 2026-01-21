// Copyright 2024 The Obsidian Authors
// This file is part of the Obsidian library.

package types

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// CallArgs represents the arguments to an eth_call or eth_estimateGas
type CallArgs struct {
	From     *common.Address `json:"from"`
	To       *common.Address `json:"to"`
	Gas      *hexutil.Uint64 `json:"gas"`
	GasPrice *hexutil.Big    `json:"gasPrice"`
	Value    *hexutil.Big    `json:"value"`
	Data     *hexutil.Bytes  `json:"data"`
	Nonce    *hexutil.Uint64 `json:"nonce"`
}

// FilterQuery represents log filter criteria
type FilterQuery struct {
	BlockHash *common.Hash     `json:"blockHash"`
	FromBlock *big.Int         `json:"fromBlock"`
	ToBlock   *big.Int         `json:"toBlock"`
	Addresses []common.Address `json:"address"`
	Topics    [][]common.Hash  `json:"topics"`
}
