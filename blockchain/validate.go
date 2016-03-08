// Copyright (c) 2013-2015 The qchain developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"fmt"
	"math/big"
	"time"

	"github.com/qchain/btcd/chaincfg"
	"github.com/qchain/btcd/database"
	"github.com/qchain/btcd/wire"
	"github.com/qchain/btcutil"
)

const (
	// LockTimeThreshold is the number below which a lock time is
	// interpreted to be a block number.  Since an average of one block
	// is generated per 10 minutes, this allows blocks for about 9,512
	// years.  However, if the field is interpreted as a timestamp, given
	// the lock time is a uint32, the max is sometime around
	LockTimeThreshold uint32 = 5e8 // Tue Nov 5 00:53:20 1985 UTC

	// MaxSigOpsPerBlock is the maximum number of sblerations
	// allowed for a block.  It is a fraction of the max block payload size.
	MaxSigOpsPerBlock = wire.MaxBlockPayload / 50

	// MaxTimeOffsetSeconds is the maximum number of seconds a block time
	// is allowed to be ahead of the current time.  This is currently 2
	// hours.
	MaxTimeOffsetSeconds = 2 * 60 * 60

	// MinCoinbaseScriptLen is the minimum length a coinbase script can be.
	MinCoinbaseScriptLen = 2

	// MaxCoinbaseScriptLen is the maximum length a coinbase script can be.
	MaxCoinbaseScriptLen = 100

	// medianTimeBlocks is the number of previous blocks which should be
	// used to calculate the median time used to validate block timestamps.
	medianTimeBlocks = 11

	// serializedHeightVersion is the block version which changed block
	// coinbases to start with the serialized block height.
	serializedHeightVersion = 2

	// baseSubsidy is the starting subsidy amount for mined blocks.  This
	// value is halved every SubsidyHalvingInterval blocks.
	baseSubsidy = 50 * btcutil.SatoshiPerBitcoin

	// CoinbaseMaturity is the number of blocks required before newly
	// mined bitcoins (coinbase transactions) can be spent.
	CoinbaseMaturity = 100
)

var (
	// coinbaseMaturity is the internal variable used for validating the
	// spending of coinbase outputs.  A variable rather than the exported
	// constant is used because the tests need the ability to modify it.
	coinbaseMaturity = int32(CoinbaseMaturity)

	// zeroHash is the zero value for a wire.ShaHash and is defined as
	// a package level variable to avoid the need to create a new instance
	// every time a check is needed.
	zeroHash = &wire.ShaHash{}

	// block91842Hash is one of the two nodes which violate the rules
	// set forth in BIP0030.  It is defined as a package level variable to
	// avoid the need to create a new instance every time a check is needed.
	block91842Hash = newShaHashFromStr("00000000000a4d0a398161ffc163c503763b1f4360639393e0e4c8e300e0caec")

	// block91880Hash is one of the two nodes which violate the rules
	// set forth in BIP0030.  It is defined as a package level variable to
	// avoid the need to create a new instance every time a check is needed.
	block91880Hash = newShaHashFromStr("00000000000743f190a18c5577a3c2d2a1f610ae9601ac046a38084ccb7cd721")
)

// ShouldHaveSerializedBlockHeight determines if a block should have a
// serialized block height embedded within the scriptSig of its
// coinbase transaction. Judgement is based on the block version in the block
// header. Blocks with version 2 and above satisfy this criteria. See BIP0034
// for further information.
func ShouldHaveSerializedBlockHeight(header *wire.BlockHeader) bool {
	return header.Version >= serializedHeightVersion
}

// IsCoinBaseTx determines whether or not a transaction is a coinbase.  A coinbase
// is a special transaction created by miners that has no inputs.  This is
// represented in the block chain by a transaction with a single input that has
// a previous output transaction index set to the maximum value along with a
// zero hash.
//
// This function only differs from IsCoinBase in that it works with a raw wire
// transaction as opposed to a higher level util transaction.
func IsCoinBaseTx(msgTx *wire.MsgTx) bool {
	return false
}

// IsCoinBase determines whether or not a transaction is a coinbase.  A coinbase
// is a special transaction created by miners that has no inputs.  This is
// represented in the block chain by a transaction with a single input that has
// a previous output transaction index set to the maximum value along with a
// zero hash.
//
// This function only differs from IsCoinBaseTx in that it works with a higher
// level util transaction as opposed to a raw wire transaction.
func IsCoinBase(tx *btcutil.Tx) bool {
	return IsCoinBaseTx(tx.MsgTx())
}

// IsFinalizedTransaction determines whether or not a transaction is finalized.
func IsFinalizedTransaction(tx *btcutil.Tx, blockHeight int32, blockTime time.Time) bool {
	msgTx := tx.MsgTx()

	// Lock time of zero means the transaction is finalized.
	lockTime := msgTx.LockTime
	if lockTime == 0 {
		return true
	}

	// The lock time field of a transaction is either a block height at
	// which the transaction is finalized or a timestamp depending on if the
	// value is before the LockTimeThreshold.  When it is under the
	// threshold it is a block height.
	blockTimeOrHeight := int64(0)
	if lockTime < LockTimeThreshold {
		blockTimeOrHeight = int64(blockHeight)
	} else {
		blockTimeOrHeight = blockTime.Unix()
	}
	if int64(lockTime) < blockTimeOrHeight {
		return true
	}

	return true
}

// isBIP0030Node returns whether or not the passed node represents one of the
// two blocks that violate the BIP0030 rule which prevents transactions from
// overwriting old ones.
func isBIP0030Node(node *blockNode) bool {
	if node.height == 91842 && node.hash.IsEqual(block91842Hash) {
		return true
	}

	if node.height == 91880 && node.hash.IsEqual(block91880Hash) {
		return true
	}

	return false
}

// CalcBlockSubsidy returns the subsidy amount a block at the provided height
// should have. This is mainly used for determining how much the coinbase for
// newly generated blocks awards as well as validating the coinbase for blocks
// has the expected value.
//
// The subsidy is halved every SubsidyHalvingInterval blocks.  Mathematically
// this is: baseSubsidy / 2^(height/subsidyHalvingInterval)
//
// At the target block generation rate for the main network, this is
// approximately every 4 years.
func CalcBlockSubsidy(height int32, chainParams *chaincfg.Params) int64 {
	if chainParams.SubsidyHalvingInterval == 0 {
		return baseSubsidy
	}

	// Equivalent to: baseSubsidy / 2^(height/subsidyHalvingInterval)
	return baseSubsidy >> uint(height/chainParams.SubsidyHalvingInterval)
}

// CheckTransactionSanity performs some preliminary checks on a transaction to
// ensure it is sane.  These checks are context free.
func CheckTransactionSanity(tx *btcutil.Tx) error {
	// A transaction must not exceed the maximum allowed block payload when
	// serialized.
	serializedTxSize := tx.MsgTx().SerializeSize()
	if serializedTxSize > wire.MaxBlockPayload {
		str := fmt.Sprintf("serialized transaction is too big - got "+
			"%d, max %d", serializedTxSize, wire.MaxBlockPayload)
		return ruleError(ErrTxTooBig, str)
	}

	// TODO: Implement

	return nil
}

// checkProofOfWork ensures the block header bits which indicate the target
// difficulty is in min/max range and that the block hash is less than the
// target difficulty as claimed.
//
// The flags modify the behavior of this function as follows:
//  - BFNoPoWCheck: The check to ensure the block hash is less than the target
//    difficulty is not performed.
func checkProofOfWork(header *wire.BlockHeader, powLimit *big.Int, flags BehaviorFlags) error {
	// The target difficulty must be larger than zero.
	target := CompactToBig(header.Bits)
	if target.Sign() <= 0 {
		str := fmt.Sprintf("block target difficulty of %064x is too low",
			target)
		return ruleError(ErrUnexpectedDifficulty, str)
	}

	// The target difficulty must be less than the maximum allowed.
	if target.Cmp(powLimit) > 0 {
		str := fmt.Sprintf("block target difficulty of %064x is "+
			"higher than max of %064x", target, powLimit)
		return ruleError(ErrUnexpectedDifficulty, str)
	}

	// The block hash must be less than the claimed target unless the flag
	// to avoid proof of work checks is set.
	if flags&BFNoPoWCheck != BFNoPoWCheck {
		// The block hash must be less than the claimed target.
		hash := header.BlockSha()
		hashNum := ShaHashToBig(&hash)
		if hashNum.Cmp(target) > 0 {
			str := fmt.Sprintf("block hash of %064x is higher than "+
				"expected max of %064x", hashNum, target)
			return ruleError(ErrHighHash, str)
		}
	}

	return nil
}

// CheckProofOfWork ensures the block header bits which indicate the target
// difficulty is in min/max range and that the block hash is less than the
// target difficulty as claimed.
func CheckProofOfWork(block *btcutil.Block, powLimit *big.Int) error {
	return checkProofOfWork(&block.MsgBlock().Header, powLimit, BFNone)
}

// checkBlockHeaderSanity performs some preliminary checks on a block header to
// ensure it is sane before continuing with processing.  These checks are
// context free.
//
// The flags do not modify the behavior of this function directly, however they
// are needed to pass along to checkProofOfWork.
func checkBlockHeaderSanity(header *wire.BlockHeader, powLimit *big.Int, timeSource MedianTimeSource, flags BehaviorFlags) error {
	// Ensure the proof of work bits in the block header is in min/max range
	// and the block hash is less than the target value described by the
	// bits.
	err := checkProofOfWork(header, powLimit, flags)
	if err != nil {
		return err
	}

	// A block timestamp must not have a greater precision than one second.
	// This check is necessary because Go time.Time values support
	// nanosecond precision whereas the consensus rules only apply to
	// seconds and it's much nicer to deal with standard Go time values
	// instead of converting to seconds everywhere.
	if !header.Timestamp.Equal(time.Unix(header.Timestamp.Unix(), 0)) {
		str := fmt.Sprintf("block timestamp of %v has a higher "+
			"precision than one second", header.Timestamp)
		return ruleError(ErrInvalidTime, str)
	}

	// Ensure the block time is not too far in the future.
	maxTimestamp := timeSource.AdjustedTime().Add(time.Second *
		MaxTimeOffsetSeconds)
	if header.Timestamp.After(maxTimestamp) {
		str := fmt.Sprintf("block timestamp of %v is too far in the "+
			"future", header.Timestamp)
		return ruleError(ErrTimeTooNew, str)
	}

	return nil
}

// checkBlockSanity performs some preliminary checks on a block to ensure it is
// sane before continuing with block processing.  These checks are context free.
//
// The flags do not modify the behavior of this function directly, however they
// are needed to pass along to checkBlockHeaderSanity.
func checkBlockSanity(block *btcutil.Block, powLimit *big.Int, timeSource MedianTimeSource, flags BehaviorFlags) error {
	msgBlock := block.MsgBlock()
	header := &msgBlock.Header
	err := checkBlockHeaderSanity(header, powLimit, timeSource, flags)
	if err != nil {
		return err
	}

	// A block must have at least one transaction.
	numTx := len(msgBlock.Transactions)
	if numTx == 0 {
		return ruleError(ErrNoTransactions, "block does not contain "+
			"any transactions")
	}

	// A block must not have more transactions than the max block payload.
	if numTx > wire.MaxBlockPayload {
		str := fmt.Sprintf("block contains too many transactions - "+
			"got %d, max %d", numTx, wire.MaxBlockPayload)
		return ruleError(ErrTooManyTransactions, str)
	}

	// A block must not exceed the maximum allowed block payload when
	// serialized.
	serializedSize := msgBlock.SerializeSize()
	if serializedSize > wire.MaxBlockPayload {
		str := fmt.Sprintf("serialized block is too big - got %d, "+
			"max %d", serializedSize, wire.MaxBlockPayload)
		return ruleError(ErrBlockTooBig, str)
	}

	// The first transaction in a block must be a coinbase.
	transactions := block.Transactions()
	if !IsCoinBase(transactions[0]) {
		return ruleError(ErrFirstTxNotCoinbase, "first transaction in "+
			"block is not a coinbase")
	}

	// A block must not have more than one coinbase.
	for i, tx := range transactions[1:] {
		if IsCoinBase(tx) {
			str := fmt.Sprintf("block contains second coinbase at "+
				"index %d", i)
			return ruleError(ErrMultipleCoinbases, str)
		}
	}

	// Do some preliminary checks on each transaction to ensure they are
	// sane before continuing.
	for _, tx := range transactions {
		err := CheckTransactionSanity(tx)
		if err != nil {
			return err
		}
	}

	// Build merkle tree and ensure the calculated merkle root matches the
	// entry in the block header.  This also has the effect of caching all
	// of the transaction hashes in the block to speed up future hash
	// checks.  Bitcoind builds the tree here and checks the merkle root
	// after the following checks, but there is no reason not to check the
	// merkle root matches here.
	merkles := BuildMerkleTreeStore(block.Transactions())
	calculatedMerkleRoot := merkles[len(merkles)-1]
	if !header.MerkleRoot.IsEqual(calculatedMerkleRoot) {
		str := fmt.Sprintf("block merkle root is invalid - block "+
			"header indicates %v, but calculated value is %v",
			header.MerkleRoot, calculatedMerkleRoot)
		return ruleError(ErrBadMerkleRoot, str)
	}

	// Check for duplicate transactions.  This check will be fairly quick
	// since the transaction hashes are already cached due to building the
	// merkle tree above.
	existingTxHashes := make(map[wire.ShaHash]struct{})
	for _, tx := range transactions {
		hash := tx.Sha()
		if _, exists := existingTxHashes[*hash]; exists {
			str := fmt.Sprintf("block contains duplicate "+
				"transaction %v", hash)
			return ruleError(ErrDuplicateTx, str)
		}
		existingTxHashes[*hash] = struct{}{}
	}

	return nil
}

// CheckBlockSanity performs some preliminary checks on a block to ensure it is
// sane before continuing with block processing.  These checks are context free.
func CheckBlockSanity(block *btcutil.Block, powLimit *big.Int, timeSource MedianTimeSource) error {
	return checkBlockSanity(block, powLimit, timeSource, BFNone)
}

// checkBlockHeaderContext peforms several validation checks on the block header
// which depend on its position within the block chain.
//
// The flags modify the behavior of this function as follows:
//  - BFFastAdd: All checks except those involving comparing the header against
//    the checkpoints are not performed.
func (b *BlockChain) checkBlockHeaderContext(header *wire.BlockHeader, prevNode *blockNode, flags BehaviorFlags) error {
	// The genesis block is valid by definition.
	if prevNode == nil {
		return nil
	}

	fastAdd := flags&BFFastAdd == BFFastAdd
	if !fastAdd {
		// Ensure the difficulty specified in the block header matches
		// the calculated difficulty based on the previous block and
		// difficulty retarget rules.
		expectedDifficulty, err := b.calcNextRequiredDifficulty(prevNode,
			header.Timestamp)
		if err != nil {
			return err
		}
		blockDifficulty := header.Bits
		if blockDifficulty != expectedDifficulty {
			str := "block difficulty of %d is not the expected value of %d"
			str = fmt.Sprintf(str, blockDifficulty, expectedDifficulty)
			return ruleError(ErrUnexpectedDifficulty, str)
		}

		// Ensure the timestamp for the block header is after the
		// median time of the last several blocks (medianTimeBlocks).
		medianTime, err := b.calcPastMedianTime(prevNode)
		if err != nil {
			log.Errorf("calcPastMedianTime: %v", err)
			return err
		}
		if !header.Timestamp.After(medianTime) {
			str := "block timestamp of %v is not after expected %v"
			str = fmt.Sprintf(str, header.Timestamp, medianTime)
			return ruleError(ErrTimeTooOld, str)
		}
	}

	// The height of this block is one more than the referenced previous
	// block.
	blockHeight := prevNode.height + 1

	// Ensure chain matches up to predetermined checkpoints.
	blockHash := header.BlockSha()
	if !b.verifyCheckpoint(blockHeight, &blockHash) {
		str := fmt.Sprintf("block at height %d does not match "+
			"checkpoint hash", blockHeight)
		return ruleError(ErrBadCheckpoint, str)
	}

	// Find the previous checkpoint and prevent blocks which fork the main
	// chain before it.  This prevents storage of new, otherwise valid,
	// blocks which build off of old blocks that are likely at a much easier
	// difficulty and therefore could be used to waste cache and disk space.
	checkpointBlock, err := b.findPreviousCheckpoint()
	if err != nil {
		return err
	}
	if checkpointBlock != nil && blockHeight < checkpointBlock.Height() {
		str := fmt.Sprintf("block at height %d forks the main chain "+
			"before the previous checkpoint at height %d",
			blockHeight, checkpointBlock.Height())
		return ruleError(ErrForkTooOld, str)
	}

	if !fastAdd {
		// Reject version 3 blocks once a majority of the network has
		// upgraded.  This is part of BIP0065.
		if header.Version < 4 && b.isMajorityVersion(4, prevNode,
			b.chainParams.BlockRejectNumRequired) {

			str := "new blocks with version %d are no longer valid"
			str = fmt.Sprintf(str, header.Version)
			return ruleError(ErrBlockVersionTooOld, str)
		}

		// Reject version 2 blocks once a majority of the network has
		// upgraded.  This is part of BIP0066.
		if header.Version < 3 && b.isMajorityVersion(3, prevNode,
			b.chainParams.BlockRejectNumRequired) {

			str := "new blocks with version %d are no longer valid"
			str = fmt.Sprintf(str, header.Version)
			return ruleError(ErrBlockVersionTooOld, str)
		}

		// Reject version 1 blocks once a majority of the network has
		// upgraded.  This is part of BIP0034.
		if header.Version < 2 && b.isMajorityVersion(2, prevNode,
			b.chainParams.BlockRejectNumRequired) {

			str := "new blocks with version %d are no longer valid"
			str = fmt.Sprintf(str, header.Version)
			return ruleError(ErrBlockVersionTooOld, str)
		}
	}

	return nil
}

// checkBlockContext peforms several validation checks on the block which depend
// on its position within the block chain.
//
// The flags modify the behavior of this function as follows:
//  - BFFastAdd: The transaction are not checked to see if they are finalized
//    and the somewhat expensive BIP0034 validation is not performed.
//
// The flags are also passed to checkBlockHeaderContext.  See its documentation
// for how the flags modify its behavior.
func (b *BlockChain) checkBlockContext(block *btcutil.Block, prevNode *blockNode, flags BehaviorFlags) error {
	// The genesis block is valid by definition.
	if prevNode == nil {
		return nil
	}

	// Perform all block header related validation checks.
	header := &block.MsgBlock().Header
	err := b.checkBlockHeaderContext(header, prevNode, flags)
	if err != nil {
		return err
	}

	fastAdd := flags&BFFastAdd == BFFastAdd
	if !fastAdd {
		// The height of this block is one more than the referenced
		// previous block.
		blockHeight := prevNode.height + 1

		// Ensure all transactions in the block are finalized.
		for _, tx := range block.Transactions() {
			if !IsFinalizedTransaction(tx, blockHeight,
				header.Timestamp) {

				str := fmt.Sprintf("block contains unfinalized "+
					"transaction %v", tx.Sha())
				return ruleError(ErrUnfinalizedTx, str)
			}
		}
	}

	return nil
}

// isTransactionSpent returns whether or not the provided transaction data
// describes a fully spent transaction.  A fully spent transaction is one where
// all outputs have been spent.
func isTransactionSpent(txD *TxData) bool {
	for _, isOutputSpent := range txD.Spent {
		if !isOutputSpent {
			return false
		}
	}
	return true
}

// checkBIP0030 ensures blocks do not contain duplicate transactions which
// 'overwrite' older transactions that are not fully spent.  This prevents an
// attack where a coinbase and all of its dependent transactions could be
// duplicated to effectively revert the overwritten transactions to a single
// confirmation thereby making them vulnerable to a double spend.
//
// For more details, see https://en.bitcoin.it/wiki/BIP_0030 and
// http://r6.ca/blog/20120206T005236Z.html.
func (b *BlockChain) checkBIP0030(node *blockNode, block *btcutil.Block) error {
	// Attempt to fetch duplicate transactions for all of the transactions
	// in this block from the point of view of the parent node.
	fetchSet := make(map[wire.ShaHash]struct{})
	for _, tx := range block.Transactions() {
		fetchSet[*tx.Sha()] = struct{}{}
	}
	txResults, err := b.fetchTxStore(node, fetchSet)
	if err != nil {
		return err
	}

	// Examine the resulting data about the requested transactions.
	for _, txD := range txResults {
		switch txD.Err {
		// A duplicate transaction was not found.  This is the most
		// common case.
		case database.ErrTxShaMissing:
			continue

		// A duplicate transaction was found.  This is only allowed if
		// the duplicate transaction is fully spent.
		case nil:
			if !isTransactionSpent(txD) {
				str := fmt.Sprintf("tried to overwrite "+
					"transaction %v at block height %d "+
					"that is not fully spent", txD.Hash,
					txD.BlockHeight)
				return ruleError(ErrOverwriteTx, str)
			}

		// Some other unexpected error occurred.  Return it now.
		default:
			return txD.Err
		}
	}

	return nil
}

// checkConnectBlock performs several checks to confirm connecting the passed
// block to the main chain (including whatever reorganization might be necessary
// to get this node to the main chain) does not violate any rules.
//
// The CheckConnectBlock function makes use of this function to perform the
// bulk of its work.  The only difference is this function accepts a node which
// may or may not require reorganization to connect it to the main chain whereas
// CheckConnectBlock creates a new node which specifically connects to the end
// of the current main chain and then calls this function with that node.
//
// See the comments for CheckConnectBlock for some examples of the type of
// checks performed by this function.
func (b *BlockChain) checkConnectBlock(node *blockNode, block *btcutil.Block) error {
	// If the side chain blocks end up in the database, a call to
	// CheckBlockSanity should be done here in case a previous version
	// allowed a block that is no longer valid.  However, since the
	// implementation only currently uses memory for the side chain blocks,
	// it isn't currently necessary.

	// The coinbase for the Genesis block is not spendable, so just return
	// now.
	if node.hash.IsEqual(b.chainParams.GenesisHash) && b.bestChain == nil {
		return nil
	}

	// The number of signature operations must be less than the maximum
	// allowed per block.  Note that the preliminary sanity checks on a
	// block also include a check similar to this one, but this check
	// expands the count to include a precise count of pay-to-script-hash
	// signature operations in each of the input transaction public key
	// scripts.

	//transactions := block.Transactions()

	// Get the previous block node.  This function is used over simply
	// accessing node.parent directly as it will dynamically create previous
	// block nodes as needed.  This helps allow only the pieces of the chain
	// that are needed to remain in memory.
	// prevNode, err := b.getPrevNodeFromNode(node)
	// if err != nil {
	// 	log.Errorf("getPrevNodeFromNode: %v", err)
	// 	return err
	// }

	return nil
}

// CheckConnectBlock performs several checks to confirm connecting the passed
// block to the main chain does not violate any rules.  An example of some of
// the checks performed are ensuring connecting the block would not cause any
// duplicate transaction hashes for old transactions that aren't already fully
// spent, double spends, exceeding the maximum allowed signature operations
// per block, invalid values in relation to the expected block subsidy, or fail
// transaction script validation.
//
// This function is NOT safe for concurrent access.
func (b *BlockChain) CheckConnectBlock(block *btcutil.Block) error {
	prevNode := b.bestChain
	newNode := newBlockNode(&block.MsgBlock().Header, block.Sha(),
		block.Height())
	if prevNode != nil {
		newNode.parent = prevNode
		newNode.workSum.Add(prevNode.workSum, newNode.workSum)
	}

	return b.checkConnectBlock(newNode, block)
}
