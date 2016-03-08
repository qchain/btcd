// Copyright (c) 2013-2015 The qchain developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"github.com/qchain/btcd/blockchain"
	"github.com/qchain/btcd/wire"
	"github.com/qchain/btcutil"
)

const (
	// maxStandardTxSize is the maximum size allowed for transactions that
	// are considered standard and will therefore be relayed and considered
	// for mining.
	maxStandardTxSize = 100000

	// maxStandardSigScriptSize is the maximum size allowed for a
	// transaction input signature script to be considered standard.  This
	// value allows for a 15-of-15 CHECKMULTISIG pay-to-script-hash with
	// compressed keys.
	//
	// The form of the overall script is: OP_0 <15 signatures> OP_PUSHDATA2
	// <2 bytes len> [OP_15 <15 pubkeys> OP_15 OP_CHECKMULTISIG]
	//
	// For the p2sh script portion, each of the 15 compressed pubkeys are
	// 33 bytes (plus one for the OP_DATA_33 opcode), and the thus it totals
	// to (15*34)+3 = 513 bytes.  Next, each of the 15 signatures is a max
	// of 73 bytes (plus one for the OP_DATA_73 opcode).  Also, there is one
	// extra byte for the initial extra OP_0 push and 3 bytes for the
	// OP_PUSHDATA2 needed to specify the 513 bytes for the script push.
	// That brings the total to 1+(15*74)+3+513 = 1627.  This value also
	// adds a few extra bytes to provide a little buffer.
	// (1 + 15*74 + 3) + (15*34 + 3) + 23 = 1650
	maxStandardSigScriptSize = 1650

	// defaultMinRelayTxFee is the minimum fee in satoshi that is required
	// for a transaction to be treated as free for relay and mining
	// purposes.  It is also used to help determine if a transaction is
	// considered dust and as a base for calculating minimum required fees
	// for larger transactions.  This value is in Satoshi/1000 bytes.
	defaultMinRelayTxFee = btcutil.Amount(1000)

	// maxStandardMultiSigKeys is the maximum number of public keys allowed
	// in a multi-signature transaction output script for it to be
	// considered standard.
	maxStandardMultiSigKeys = 3
)

// calcMinRequiredTxRelayFee returns the minimum transaction fee required for a
// transaction with the passed serialized size to be accepted into the memory
// pool and relayed.
func calcMinRequiredTxRelayFee(serializedSize int64, minRelayTxFee btcutil.Amount) int64 {
	// Calculate the minimum fee for a transaction to be allowed into the
	// mempool and relayed by scaling the base fee (which is the minimum
	// free transaction relay fee). minTxRelayFee is in Satoshi/kB so
	// multiply by serializedSize (which is in bytes) and divide by 1000 to get
	// minimum Satoshis.
	minFee := (serializedSize * int64(minRelayTxFee)) / 1000

	if minFee == 0 && minRelayTxFee > 0 {
		minFee = int64(minRelayTxFee)
	}

	// Set the minimum fee to the maximum possible value if the calculated
	// fee is not in the valid range for monetary amounts.
	if minFee < 0 || minFee > btcutil.MaxSatoshi {
		minFee = btcutil.MaxSatoshi
	}

	return minFee
}

// calcPriority returns a transaction priority given a transaction and the sum
// of each of its input values multiplied by their age (# of confirmations).
// Thus, the final formula for the priority is:
// sum(inputValue * inputAge) / adjustedTxSize
func calcPriority(tx *wire.MsgTx, txStore blockchain.TxStore, nextBlockHeight int32) float64 {
	// In order to encourage spending multiple old unspent transaction
	// outputs thereby reducing the total set, don't count the constant
	// overhead for each input as well as enough bytes of the signature
	// script to cover a pay-to-script-hash redemption with a compressed
	// pubkey.  This makes additional inputs free by boosting the priority
	// of the transaction accordingly.  No more incentive is given to avoid
	// encouraging gaming future transactions through the use of junk
	// outputs.  This is the same logic used in the reference
	// implementation.
	//
	// The constant overhead for a txin is 41 bytes since the previous
	// outpoint is 36 bytes + 4 bytes for the sequence + 1 byte the
	// signature script length.
	//
	// A compressed pubkey pay-to-script-hash redemption with a maximum len
	// signature is of the form:
	// [OP_DATA_73 <73-byte sig> + OP_DATA_35 + {OP_DATA_33
	// <33 byte compresed pubkey> + OP_CHECKSIG}]
	//
	// Thus 1 + 73 + 1 + 1 + 33 + 1 = 110
	overhead := 0

	serializedTxSize := tx.SerializeSize()
	if overhead >= serializedTxSize {
		return 0.0
	}

	inputValueAge := calcInputValueAge(tx, txStore, nextBlockHeight)
	return inputValueAge / float64(serializedTxSize-overhead)
}

// calcInputValueAge is a helper function used to calculate the input age of
// a transaction.  The input age for a txin is the number of confirmations
// since the referenced txout multiplied by its output value.  The total input
// age is the sum of this value for each txin.  Any inputs to the transaction
// which are currently in the mempool and hence not mined into a block yet,
// contribute no additional input age to the transaction.
func calcInputValueAge(tx *wire.MsgTx, txStore blockchain.TxStore, nextBlockHeight int32) float64 {
	// TODO: Implement

	return float64(0.0)
}

// checkTransactionStandard performs a series of checks on a transaction to
// ensure it is a "standard" transaction.  A standard transaction is one that
// conforms to several additional limiting cases over what is considered a
// "sane" transaction such as having a version in the supported range, being
// finalized, conforming to more stringent size constraints, having scripts
// of recognized forms, and not containing "dust" outputs (those that are
// so small it costs more to process them than they are worth).
func checkTransactionStandard(tx *btcutil.Tx, height int32, timeSource blockchain.MedianTimeSource, minRelayTxFee btcutil.Amount) error {
	// TODO: Implement

	return nil
}

// minInt is a helper function to return the minimum of two ints.  This avoids
// a math import and the need to cast to floats.
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
