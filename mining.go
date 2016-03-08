// Copyright (c) 2014 The qchain developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"container/heap"
	"container/list"
	"time"

	"github.com/qchain/btcd/blockchain"
	"github.com/qchain/btcd/database"
	"github.com/qchain/btcd/mining"
	"github.com/qchain/btcd/wire"
	"github.com/qchain/btcutil"
)

const (
	// generatedBlockVersion is the version of the block being generated.
	// It is defined as a constant here rather than using the
	// wire.BlockVersion constant since a change in the block version
	// will require changes to the generated block.  Using the wire constant
	// for generated block version could allow creation of invalid blocks
	// for the updated version.
	generatedBlockVersion = 4

	// minHighPriority is the minimum priority value that allows a
	// transaction to be considered high priority.
	minHighPriority = btcutil.SatoshiPerBitcoin * 144.0 / 250

	// blockHeaderOverhead is the max number of bytes it takes to serialize
	// a block header and max possible transaction count.
	blockHeaderOverhead = wire.MaxBlockHeaderPayload + wire.MaxVarIntPayload

	// coinbaseFlags is added to the coinbase script of a generated block
	// and is used to monitor BIP16 support as well as blocks that are
	// generated via btcd.
	coinbaseFlags = "/P2SH/btcd/"
)

// txPrioItem houses a transaction along with extra information that allows the
// transaction to be prioritized and track dependencies on other transactions
// which have not been mined into a block yet.
type txPrioItem struct {
	tx       *btcutil.Tx
	fee      int64
	priority float64
	feePerKB int64

	// dependsOn holds a map of transaction hashes which this one depends
	// on.  It will only be set when the transaction references other
	// transactions in the source pool and hence must come after them in
	// a block.
	dependsOn map[wire.ShaHash]struct{}
}

// txPriorityQueueLessFunc describes a function that can be used as a compare
// function for a transaction priority queue (txPriorityQueue).
type txPriorityQueueLessFunc func(*txPriorityQueue, int, int) bool

// txPriorityQueue implements a priority queue of txPrioItem elements that
// supports an arbitrary compare function as defined by txPriorityQueueLessFunc.
type txPriorityQueue struct {
	lessFunc txPriorityQueueLessFunc
	items    []*txPrioItem
}

// Len returns the number of items in the priority queue.  It is part of the
// heap.Interface implementation.
func (pq *txPriorityQueue) Len() int {
	return len(pq.items)
}

// Less returns whether the item in the priority queue with index i should sort
// before the item with index j by deferring to the assigned less function.  It
// is part of the heap.Interface implementation.
func (pq *txPriorityQueue) Less(i, j int) bool {
	return pq.lessFunc(pq, i, j)
}

// Swap swaps the items at the passed indices in the priority queue.  It is
// part of the heap.Interface implementation.
func (pq *txPriorityQueue) Swap(i, j int) {
	pq.items[i], pq.items[j] = pq.items[j], pq.items[i]
}

// Push pushes the passed item onto the priority queue.  It is part of the
// heap.Interface implementation.
func (pq *txPriorityQueue) Push(x interface{}) {
	pq.items = append(pq.items, x.(*txPrioItem))
}

// Pop removes the highest priority item (according to Less) from the priority
// queue and returns it.  It is part of the heap.Interface implementation.
func (pq *txPriorityQueue) Pop() interface{} {
	n := len(pq.items)
	item := pq.items[n-1]
	pq.items[n-1] = nil
	pq.items = pq.items[0 : n-1]
	return item
}

// SetLessFunc sets the compare function for the priority queue to the provided
// function.  It also invokes heap.Init on the priority queue using the new
// function so it can immediately be used with heap.Push/Pop.
func (pq *txPriorityQueue) SetLessFunc(lessFunc txPriorityQueueLessFunc) {
	pq.lessFunc = lessFunc
	heap.Init(pq)
}

// txPQByPriority sorts a txPriorityQueue by transaction priority and then fees
// per kilobyte.
func txPQByPriority(pq *txPriorityQueue, i, j int) bool {
	// Using > here so that pop gives the highest priority item as opposed
	// to the lowest.  Sort by priority first, then fee.
	if pq.items[i].priority == pq.items[j].priority {
		return pq.items[i].feePerKB > pq.items[j].feePerKB
	}
	return pq.items[i].priority > pq.items[j].priority

}

// txPQByFee sorts a txPriorityQueue by fees per kilobyte and then transaction
// priority.
func txPQByFee(pq *txPriorityQueue, i, j int) bool {
	// Using > here so that pop gives the highest fee item as opposed
	// to the lowest.  Sort by fee first, then priority.
	if pq.items[i].feePerKB == pq.items[j].feePerKB {
		return pq.items[i].priority > pq.items[j].priority
	}
	return pq.items[i].feePerKB > pq.items[j].feePerKB
}

// newTxPriorityQueue returns a new transaction priority queue that reserves the
// passed amount of space for the elements.  The new priority queue uses either
// the txPQByPriority or the txPQByFee compare function depending on the
// sortByFee parameter and is already initialized for use with heap.Push/Pop.
// The priority queue can grow larger than the reserved space, but extra copies
// of the underlying array can be avoided by reserving a sane value.
func newTxPriorityQueue(reserve int, sortByFee bool) *txPriorityQueue {
	pq := &txPriorityQueue{
		items: make([]*txPrioItem, 0, reserve),
	}
	if sortByFee {
		pq.SetLessFunc(txPQByFee)
	} else {
		pq.SetLessFunc(txPQByPriority)
	}
	return pq
}

// BlockTemplate houses a block that has yet to be solved along with additional
// details about the fees and the number of signature operations for each
// transaction in the block.
type BlockTemplate struct {
	block           *wire.MsgBlock
	fees            []int64
	sigOpCounts     []int64
	height          int32
	validPayAddress bool
}

// mergeTxStore adds all of the transactions in txStoreB to txStoreA.  The
// result is that txStoreA will contain all of its original transactions plus
// all of the transactions in txStoreB.
func mergeTxStore(txStoreA blockchain.TxStore, txStoreB blockchain.TxStore) {
	for hash, txDataB := range txStoreB {
		if txDataA, exists := txStoreA[hash]; !exists ||
			(txDataA.Err == database.ErrTxShaMissing &&
				txDataB.Err != database.ErrTxShaMissing) {

			txStoreA[hash] = txDataB
		}
	}
}

// logSkippedDeps logs any dependencies which are also skipped as a result of
// skipping a transaction while generating a block template at the trace level.
func logSkippedDeps(tx *btcutil.Tx, deps *list.List) {
	if deps == nil {
		return
	}

	for e := deps.Front(); e != nil; e = e.Next() {
		item := e.Value.(*txPrioItem)
		minrLog.Tracef("Skipping tx %s since it depends on %s\n",
			item.tx.Sha(), tx.Sha())
	}
}

// minimumMedianTime returns the minimum allowed timestamp for a block building
// on the end of the current best chain.  In particular, it is one second after
// the median timestamp of the last several blocks per the chain consensus
// rules.
func minimumMedianTime(chainState *chainState) (time.Time, error) {
	chainState.Lock()
	defer chainState.Unlock()
	if chainState.pastMedianTimeErr != nil {
		return time.Time{}, chainState.pastMedianTimeErr
	}

	return chainState.pastMedianTime.Add(time.Second), nil
}

// medianAdjustedTime returns the current time adjusted to ensure it is at least
// one second after the median timestamp of the last several blocks per the
// chain consensus rules.
func medianAdjustedTime(chainState *chainState, timeSource blockchain.MedianTimeSource) (time.Time, error) {
	chainState.Lock()
	defer chainState.Unlock()
	if chainState.pastMedianTimeErr != nil {
		return time.Time{}, chainState.pastMedianTimeErr
	}

	// The timestamp for the block must not be before the median timestamp
	// of the last several blocks.  Thus, choose the maximum between the
	// current time and one second after the past median time.  The current
	// timestamp is truncated to a second boundary before comparison since a
	// block timestamp does not supported a precision greater than one
	// second.
	newTimestamp := timeSource.AdjustedTime()
	minTimestamp := chainState.pastMedianTime.Add(time.Second)
	if newTimestamp.Before(minTimestamp) {
		newTimestamp = minTimestamp
	}

	return newTimestamp, nil
}

// NewBlockTemplate returns a new block template that is ready to be solved
// using the transactions from the passed transaction source pool and a coinbase
// that either pays to the passed address if it is not nil, or a coinbase that
// is redeemable by anyone if the passed address is nil.  The nil address
// functionality is useful since there are cases such as the getblocktemplate
// RPC where external mining software is responsible for creating their own
// coinbase which will replace the one generated for the block template.  Thus
// the need to have configured address can be avoided.
//
// The transactions selected and included are prioritized according to several
// factors.  First, each transaction has a priority calculated based on its
// value, age of inputs, and size.  Transactions which consist of larger
// amounts, older inputs, and small sizes have the highest priority.  Second, a
// fee per kilobyte is calculated for each transaction.  Transactions with a
// higher fee per kilobyte are preferred.  Finally, the block generation related
// policy settings are all taken into account.
//
// Transactions which only spend outputs from other transactions already in the
// block chain are immediately added to a priority queue which either
// prioritizes based on the priority (then fee per kilobyte) or the fee per
// kilobyte (then priority) depending on whether or not the BlockPrioritySize
// policy setting allots space for high-priority transactions.  Transactions
// which spend outputs from other transactions in the source pool are added to a
// dependency map so they can be added to the priority queue once the
// transactions they depend on have been included.
//
// Once the high-priority area (if configured) has been filled with
// transactions, or the priority falls below what is considered high-priority,
// the priority queue is updated to prioritize by fees per kilobyte (then
// priority).
//
// When the fees per kilobyte drop below the TxMinFreeFee policy setting, the
// transaction will be skipped unless the BlockMinSize policy setting is
// nonzero, in which case the block will be filled with the low-fee/free
// transactions until the block size reaches that minimum size.
//
// Any transactions which would cause the block to exceed the BlockMaxSize
// policy setting, exceed the maximum allowed signature operations per block, or
// otherwise cause the block to be invalid are skipped.
//
// Given the above, a block generated by this function is of the following form:
//
//   -----------------------------------  --  --
//  |      Coinbase Transaction         |   |   |
//  |-----------------------------------|   |   |
//  |                                   |   |   | ----- policy.BlockPrioritySize
//  |   High-priority Transactions      |   |   |
//  |                                   |   |   |
//  |-----------------------------------|   | --
//  |                                   |   |
//  |                                   |   |
//  |                                   |   |--- policy.BlockMaxSize
//  |  Transactions prioritized by fee  |   |
//  |  until <= policy.TxMinFreeFee     |   |
//  |                                   |   |
//  |                                   |   |
//  |                                   |   |
//  |-----------------------------------|   |
//  |  Low-fee/Non high-priority (free) |   |
//  |  transactions (while block size   |   |
//  |  <= policy.BlockMinSize)          |   |
//   -----------------------------------  --
// TODO: Update documentation with new implementation
func NewBlockTemplate(policy *mining.Policy, server *server, payToAddress btcutil.Address) (*BlockTemplate, error) {
	// TODO: Implement this to generate based on new block template
	var txSource mining.TxSource = server.txMemPool
	blockManager := server.blockManager
	timeSource := server.timeSource
	chainState := &blockManager.chainState

	// Extend the most recently known best block.
	chainState.Lock()
	prevHash := chainState.newestHash
	nextBlockHeight := chainState.newestHeight + 1
	chainState.Unlock()

	// Get the current source transactions and create a priority queue to
	// hold the transactions which are ready for inclusion into a block
	// along with some priority related and fee metadata.  Reserve the same
	// number of items that are available for the priority queue.  Also,
	// choose the initial sort order for the priority queue based on whether
	// or not there is an area allocated for high-priority transactions.
	sourceTxns := txSource.MiningDescs()
	sortedByFee := policy.BlockPrioritySize == 0
	priorityQueue := newTxPriorityQueue(len(sourceTxns), sortedByFee)

	// Create a slice to hold the transactions to be included in the
	// generated block with reserved space.  Also create a transaction
	// store to house all of the input transactions so multiple lookups
	// can be avoided.
	blockTxns := make([]*btcutil.Tx, 0, len(sourceTxns))
	//blockTxns = append(blockTxns, coinbaseTx)
	blockTxStore := make(blockchain.TxStore)

	// dependers is used to track transactions which depend on another
	// transaction in the source pool.  This, in conjunction with the
	// dependsOn map kept with each dependent transaction helps quickly
	// determine which dependent transactions are now eligible for inclusion
	// in the block once each transaction has been included.
	dependers := make(map[wire.ShaHash]*list.List)

	// Create slices to hold the fees and number of signature operations
	// for each of the selected transactions and add an entry for the
	// coinbase.  This allows the code below to simply append details about
	// a transaction as it is selected for inclusion in the final block.
	// However, since the total fees aren't known yet, use a dummy value for
	// the coinbase fee which will be updated later.
	//txFees := make([]int64, 0, len(sourceTxns))
	//txSigOpCounts := make([]int64, 0, len(sourceTxns))
	//txFees = append(txFees, -1) // Updated once known
	//txSigOpCounts = append(txSigOpCounts, numCoinbaseSigOps)

	minrLog.Debugf("Considering %d transactions for inclusion to new block",
		len(sourceTxns))

mempoolLoop:
	for _, txDesc := range sourceTxns {
		// A block can't have more than one coinbase or contain
		// non-finalized transactions.
		tx := txDesc.Tx
		if blockchain.IsCoinBase(tx) {
			minrLog.Tracef("Skipping coinbase tx %s", tx.Sha())
			continue
		}
		if !blockchain.IsFinalizedTransaction(tx, nextBlockHeight,
			timeSource.AdjustedTime()) {

			minrLog.Tracef("Skipping non-finalized tx %s", tx.Sha())
			continue
		}

		// Fetch all of the transactions referenced by the inputs to
		// this transaction.  NOTE: This intentionally does not fetch
		// inputs from the mempool since a transaction which depends on
		// other transactions in the mempool must come after those
		// dependencies in the final generated block.
		txStore, err := blockManager.FetchTransactionStore(tx)
		if err != nil {
			minrLog.Warnf("Unable to fetch transaction store for "+
				"tx %s: %v", tx.Sha(), err)
			continue
		}

		prioItem := &txPrioItem{tx: tx}

		// Calculate the final transaction priority using the input
		// value age sum as well as the adjusted transaction size.  The
		// formula is: sum(inputValue * inputAge) / adjustedTxSize
		prioItem.priority = calcPriority(tx.MsgTx(), txStore, nextBlockHeight)

		// Calculate the fee in Satoshi/kB.
		txSize := tx.MsgTx().SerializeSize()
		prioItem.feePerKB = (txDesc.Fee * 1000) / int64(txSize)
		prioItem.fee = txDesc.Fee

		// Add the transaction to the priority queue to mark it ready
		// for inclusion in the block unless it has dependencies.
		if prioItem.dependsOn == nil {
			heap.Push(priorityQueue, prioItem)
		}

		// Merge the store which contains all of the input transactions
		// for this transaction into the input transaction store.  This
		// allows the code below to avoid a second lookup.
		mergeTxStore(blockTxStore, txStore)
	}

	minrLog.Tracef("Priority queue len %d, dependers len %d",
		priorityQueue.Len(), len(dependers))

	// The starting block size is the size of the block header plus the max
	// possible transaction count size, plus the size of the coinbase
	// transaction.
	// TODO: Calculate accurate block size
	blockSize := blockHeaderOverhead + uint32(999) //uint32(coinbaseTx.MsgTx().SerializeSize())
	//blockSigOps := numCoinbaseSigOps
	totalFees := int64(0)

	// Choose which transactions make it into the block.
	for priorityQueue.Len() > 0 {
		// Grab the highest priority (or highest fee per kilobyte
		// depending on the sort order) transaction.
		prioItem := heap.Pop(priorityQueue).(*txPrioItem)
		tx := prioItem.tx

		// Grab the list of transactions which depend on this one (if
		// any) and remove the entry for this transaction as it will
		// either be included or skipped, but in either case the deps
		// are no longer needed.
		deps := dependers[*tx.Sha()]
		delete(dependers, *tx.Sha())

		// Enforce maximum block size.  Also check for overflow.
		txSize := uint32(tx.MsgTx().SerializeSize())
		blockPlusTxSize := blockSize + txSize
		if blockPlusTxSize < blockSize || blockPlusTxSize >= policy.BlockMaxSize {
			minrLog.Tracef("Skipping tx %s because it would exceed "+
				"the max block size", tx.Sha())
			logSkippedDeps(tx, deps)
			continue
		}

		// Skip free transactions once the block is larger than the
		// minimum block size.
		if sortedByFee &&
			prioItem.feePerKB < int64(policy.TxMinFreeFee) &&
			blockPlusTxSize >= policy.BlockMinSize {

			minrLog.Tracef("Skipping tx %s with feePerKB %.2f "+
				"< TxMinFreeFee %d and block size %d >= "+
				"minBlockSize %d", tx.Sha(), prioItem.feePerKB,
				policy.TxMinFreeFee, blockPlusTxSize,
				policy.BlockMinSize)
			logSkippedDeps(tx, deps)
			continue
		}

		// Prioritize by fee per kilobyte once the block is larger than
		// the priority size or there are no more high-priority
		// transactions.
		if !sortedByFee && (blockPlusTxSize >= policy.BlockPrioritySize ||
			prioItem.priority <= minHighPriority) {

			minrLog.Tracef("Switching to sort by fees per "+
				"kilobyte blockSize %d >= BlockPrioritySize "+
				"%d || priority %.2f <= minHighPriority %.2f",
				blockPlusTxSize, policy.BlockPrioritySize,
				prioItem.priority, minHighPriority)

			sortedByFee = true
			priorityQueue.SetLessFunc(txPQByFee)

			// Put the transaction back into the priority queue and
			// skip it so it is re-priortized by fees if it won't
			// fit into the high-priority section or the priority is
			// too low.  Otherwise this transaction will be the
			// final one in the high-priority section, so just fall
			// though to the code below so it is added now.
			if blockPlusTxSize > policy.BlockPrioritySize ||
				prioItem.priority < minHighPriority {

				heap.Push(priorityQueue, prioItem)
				continue
			}
		}

		// NOTE: Remeber to do neccessary checks that any included
		// transactions are valid.

		// Add the transaction to the block, increment counters, and
		// save the fees and signature operation counts to the block
		// template.
		blockTxns = append(blockTxns, tx)
		blockSize += txSize

		minrLog.Tracef("Adding tx %s (priority %.2f, feePerKB %.2f)",
			prioItem.tx.Sha(), prioItem.priority, prioItem.feePerKB)

		// Add transactions which depend on this one (and also do not
		// have any other unsatisified dependencies) to the priority
		// queue.
		if deps != nil {
			for e := deps.Front(); e != nil; e = e.Next() {
				// Add the transaction to the priority queue if
				// there are no more dependencies after this
				// one.
				item := e.Value.(*txPrioItem)
				delete(item.dependsOn, *tx.Sha())
				if len(item.dependsOn) == 0 {
					heap.Push(priorityQueue, item)
				}
			}
		}
	}

	// Calculate the required difficulty for the block.  The timestamp
	// is potentially adjusted to ensure it comes after the median time of
	// the last several blocks per the chain consensus rules.
	ts, err := medianAdjustedTime(chainState, timeSource)
	if err != nil {
		return nil, err
	}
	requiredDifficulty, err := blockManager.CalcNextRequiredDifficulty(ts)
	if err != nil {
		return nil, err
	}

	// Create a new block ready to be solved.
	merkles := blockchain.BuildMerkleTreeStore(blockTxns)
	var msgBlock wire.MsgBlock
	msgBlock.Header = wire.BlockHeader{
		Version:    generatedBlockVersion,
		PrevBlock:  *prevHash,
		MerkleRoot: *merkles[len(merkles)-1],
		Timestamp:  ts,
		Bits:       requiredDifficulty,
	}
	for _, tx := range blockTxns {
		if err := msgBlock.AddTransaction(tx.MsgTx()); err != nil {
			return nil, err
		}
	}

	// Finally, perform a full check on the created block against the chain
	// consensus rules to ensure it properly connects to the current best
	// chain with no issues.
	block := btcutil.NewBlock(&msgBlock)
	block.SetHeight(nextBlockHeight)
	if err := blockManager.CheckConnectBlock(block); err != nil {
		return nil, err
	}

	minrLog.Debugf("Created new block template (%d transactions, %d in "+
		"fees, %d bytes, target difficulty "+
		"%064x)", len(msgBlock.Transactions), totalFees,
		blockSize, blockchain.CompactToBig(msgBlock.Header.Bits))

	return &BlockTemplate{
		block:           &msgBlock,
		height:          nextBlockHeight,
		validPayAddress: payToAddress != nil,
	}, nil
}

// UpdateBlockTime updates the timestamp in the header of the passed block to
// the current time while taking into account the median time of the last
// several blocks to ensure the new time is after that time per the chain
// consensus rules.  Finally, it will update the target difficulty if needed
// based on the new time for the test networks since their target difficulty can
// change based upon time.
func UpdateBlockTime(msgBlock *wire.MsgBlock, bManager *blockManager) error {
	// The new timestamp is potentially adjusted to ensure it comes after
	// the median time of the last several blocks per the chain consensus
	// rules.
	newTimestamp, err := medianAdjustedTime(&bManager.chainState,
		bManager.server.timeSource)
	if err != nil {
		return err
	}
	msgBlock.Header.Timestamp = newTimestamp

	// If running on a network that requires recalculating the difficulty,
	// do so now.
	if activeNetParams.ResetMinDifficulty {
		difficulty, err := bManager.CalcNextRequiredDifficulty(newTimestamp)
		if err != nil {
			return err
		}
		msgBlock.Header.Bits = difficulty
	}

	return nil
}

// UpdateExtraNonce updates the extra nonce in the coinbase script of the passed
// block by regenerating the coinbase script with the passed value and block
// height.  It also recalculates and updates the new merkle root that results
// from changing the coinbase script.
func UpdateExtraNonce(msgBlock *wire.MsgBlock, blockHeight int32, extraNonce uint64) error {
	// TODO: Update extra nouces if needed

	// TODO(davec): A btcutil.Block should use saved in the state to avoid
	// recalculating all of the other transaction hashes.
	// block.Transactions[0].InvalidateCache()

	// Recalculate the merkle root with the updated extra nonce.
	block := btcutil.NewBlock(msgBlock)
	merkles := blockchain.BuildMerkleTreeStore(block.Transactions())
	msgBlock.Header.MerkleRoot = *merkles[len(merkles)-1]
	return nil
}
