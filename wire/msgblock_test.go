// Copyright (c) 2013-2015 The qchain developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire_test

import (
	"bytes"
	"io"
	"reflect"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/qchain/btcd/wire"
)

// TestBlock tests the MsgBlock API.
func TestBlock(t *testing.T) {
	pver := wire.ProtocolVersion

	// Block 1 header.
	prevHash := &blockOne.Header.PrevBlock
	merkleHash := &blockOne.Header.MerkleRoot
	bits := blockOne.Header.Bits
	nonce := blockOne.Header.Nonce
	bh := wire.NewBlockHeader(prevHash, merkleHash, bits, nonce)

	// Ensure the command is expected value.
	wantCmd := "block"
	msg := wire.NewMsgBlock(bh)
	if cmd := msg.Command(); cmd != wantCmd {
		t.Errorf("NewMsgBlock: wrong command - got %v want %v",
			cmd, wantCmd)
	}

	// Ensure max payload is expected value for latest protocol version.
	// Num addresses (varInt) + max allowed addresses.
	wantPayload := uint32(2000000)
	maxPayload := msg.MaxPayloadLength(pver)
	if maxPayload != wantPayload {
		t.Errorf("MaxPayloadLength: wrong max payload length for "+
			"protocol version %d - got %v, want %v", pver,
			maxPayload, wantPayload)
	}

	// Ensure we get the same block header data back out.
	if !reflect.DeepEqual(&msg.Header, bh) {
		t.Errorf("NewMsgBlock: wrong block header - got %v, want %v",
			spew.Sdump(&msg.Header), spew.Sdump(bh))
	}

	// Ensure transactions are added properly.
	tx := blockOne.Transactions[0].Copy()
	msg.AddTransaction(tx)
	if !reflect.DeepEqual(msg.Transactions, blockOne.Transactions) {
		t.Errorf("AddTransaction: wrong transactions - got %v, want %v",
			spew.Sdump(msg.Transactions),
			spew.Sdump(blockOne.Transactions))
	}

	// Ensure transactions are properly cleared.
	msg.ClearTransactions()
	if len(msg.Transactions) != 0 {
		t.Errorf("ClearTransactions: wrong transactions - got %v, want %v",
			len(msg.Transactions), 0)
	}

	return
}

// TestBlockTxShas tests the ability to generate a slice of all transaction
// hashes from a block accurately.
func TestBlockTxShas(t *testing.T) {
	// Block 1, transaction 1 hash.
	hashStr := "f51d750099dfb94f356d1b21823d49a6390b0cec80089a0fd3f4fefca36c2bf3"
	wantHash, err := wire.NewShaHashFromStr(hashStr)
	if err != nil {
		t.Errorf("NewShaHashFromStr: %v", err)
		return
	}

	wantShas := []wire.ShaHash{*wantHash}
	shas, err := blockOne.TxShas()
	if err != nil {
		t.Errorf("TxShas: %v", err)
	}
	if !reflect.DeepEqual(shas, wantShas) {
		t.Errorf("TxShas: wrong transaction hashes - got %v, want %v",
			spew.Sdump(shas), spew.Sdump(wantShas))
	}
}

// TestBlockSha tests the ability to generate the hash of a block accurately.
func TestBlockSha(t *testing.T) {
	// Block 1 hash.
	hashStr := "839a8e6886ab5951d76f411475428afc90947ee320161bbf18eb6048"
	wantHash, err := wire.NewShaHashFromStr(hashStr)
	if err != nil {
		t.Errorf("NewShaHashFromStr: %v", err)
	}

	// Ensure the hash produced is expected.
	blockHash := blockOne.BlockSha()
	if !blockHash.IsEqual(wantHash) {
		t.Errorf("BlockSha: wrong hash - got %v, want %v",
			spew.Sprint(blockHash), spew.Sprint(wantHash))
	}
}

// TestBlockWire tests the MsgBlock wire encode and decode for various numbers
// of transaction inputs and outputs and protocol versions.
func TestBlockWire(t *testing.T) {
	tests := []struct {
		in     *wire.MsgBlock // Message to encode
		out    *wire.MsgBlock // Expected decoded message
		buf    []byte         // Wire encoding
		txLocs []wire.TxLoc   // Expected transaction locations
		pver   uint32         // Protocol version for wire encoding
	}{
		// Latest protocol version.
		{
			&blockOne,
			&blockOne,
			blockOneBytes,
			blockOneTxLocs,
			wire.ProtocolVersion,
		},

		// Protocol version BIP0035Version.
		{
			&blockOne,
			&blockOne,
			blockOneBytes,
			blockOneTxLocs,
			wire.BIP0035Version,
		},

		// Protocol version BIP0031Version.
		{
			&blockOne,
			&blockOne,
			blockOneBytes,
			blockOneTxLocs,
			wire.BIP0031Version,
		},

		// Protocol version NetAddressTimeVersion.
		{
			&blockOne,
			&blockOne,
			blockOneBytes,
			blockOneTxLocs,
			wire.NetAddressTimeVersion,
		},

		// Protocol version MultipleAddressVersion.
		{
			&blockOne,
			&blockOne,
			blockOneBytes,
			blockOneTxLocs,
			wire.MultipleAddressVersion,
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Encode the message to wire format.
		var buf bytes.Buffer
		err := test.in.MsgEncode(&buf, test.pver)
		if err != nil {
			t.Errorf("MsgEncode #%d error %v", i, err)
			continue
		}
		if !bytes.Equal(buf.Bytes(), test.buf) {
			t.Errorf("MsgEncode #%d\n got: %s want: %s", i,
				spew.Sdump(buf.Bytes()), spew.Sdump(test.buf))
			continue
		}

		// Decode the message from wire format.
		var msg wire.MsgBlock
		rbuf := bytes.NewReader(test.buf)
		err = msg.MsgDecode(rbuf, test.pver)
		if err != nil {
			t.Errorf("MsgDecode #%d error %v", i, err)
			continue
		}
		if !reflect.DeepEqual(&msg, test.out) {
			t.Errorf("MsgDecode #%d\n got: %s want: %s", i,
				spew.Sdump(&msg), spew.Sdump(test.out))
			continue
		}
	}
}

// TestBlockWireErrors performs negative tests against wire encode and decode
// of MsgBlock to confirm error paths work correctly.
func TestBlockWireErrors(t *testing.T) {
	// Use protocol version 60002 specifically here instead of the latest
	// because the test data is using bytes encoded with that protocol
	// version.
	pver := uint32(60002)

	tests := []struct {
		in       *wire.MsgBlock // Value to encode
		buf      []byte         // Wire encoding
		pver     uint32         // Protocol version for wire encoding
		max      int            // Max size of fixed buffer to induce errors
		writeErr error          // Expected write error
		readErr  error          // Expected read error
	}{
		// Force error in version.
		{&blockOne, blockOneBytes, pver, 0, io.ErrShortWrite, io.EOF},
		// Force error in prev block hash.
		{&blockOne, blockOneBytes, pver, 4, io.ErrShortWrite, io.EOF},
		// Force error in merkle root.
		{&blockOne, blockOneBytes, pver, 36, io.ErrShortWrite, io.EOF},
		// Force error in timestamp.
		{&blockOne, blockOneBytes, pver, 68, io.ErrShortWrite, io.EOF},
		// Force error in difficulty bits.
		{&blockOne, blockOneBytes, pver, 72, io.ErrShortWrite, io.EOF},
		// Force error in header nonce.
		{&blockOne, blockOneBytes, pver, 76, io.ErrShortWrite, io.EOF},
		// Force error in transaction count.
		{&blockOne, blockOneBytes, pver, 80, io.ErrShortWrite, io.EOF},
		// Force error in transactions.
		{&blockOne, blockOneBytes, pver, 81, io.ErrShortWrite, io.EOF},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Encode to wire format.
		w := newFixedWriter(test.max)
		err := test.in.MsgEncode(w, test.pver)
		if err != test.writeErr {
			t.Errorf("MsgEncode #%d wrong error got: %v, want: %v",
				i, err, test.writeErr)
			continue
		}

		// Decode from wire format.
		var msg wire.MsgBlock
		r := newFixedReader(test.max, test.buf)
		err = msg.MsgDecode(r, test.pver)
		if err != test.readErr {
			t.Errorf("MsgDecode #%d wrong error got: %v, want: %v",
				i, err, test.readErr)
			continue
		}
	}
}

// TestBlockSerialize tests MsgBlock serialize and deserialize.
func TestBlockSerialize(t *testing.T) {
	tests := []struct {
		in     *wire.MsgBlock // Message to encode
		out    *wire.MsgBlock // Expected decoded message
		buf    []byte         // Serialized data
		txLocs []wire.TxLoc   // Expected transaction locations
	}{
		{
			&blockOne,
			&blockOne,
			blockOneBytes,
			blockOneTxLocs,
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Serialize the block.
		var buf bytes.Buffer
		err := test.in.Serialize(&buf)
		if err != nil {
			t.Errorf("Serialize #%d error %v", i, err)
			continue
		}
		if !bytes.Equal(buf.Bytes(), test.buf) {
			t.Errorf("Serialize #%d\n got: %s want: %s", i,
				spew.Sdump(buf.Bytes()), spew.Sdump(test.buf))
			continue
		}

		// Deserialize the block.
		var block wire.MsgBlock
		rbuf := bytes.NewReader(test.buf)
		err = block.Deserialize(rbuf)
		if err != nil {
			t.Errorf("Deserialize #%d error %v", i, err)
			continue
		}
		if !reflect.DeepEqual(&block, test.out) {
			t.Errorf("Deserialize #%d\n got: %s want: %s", i,
				spew.Sdump(&block), spew.Sdump(test.out))
			continue
		}

		// Deserialize the block while gathering transaction location
		// information.
		var txLocBlock wire.MsgBlock
		br := bytes.NewBuffer(test.buf)
		txLocs, err := txLocBlock.DeserializeTxLoc(br)
		if err != nil {
			t.Errorf("DeserializeTxLoc #%d error %v", i, err)
			continue
		}
		if !reflect.DeepEqual(&txLocBlock, test.out) {
			t.Errorf("DeserializeTxLoc #%d\n got: %s want: %s", i,
				spew.Sdump(&txLocBlock), spew.Sdump(test.out))
			continue
		}
		if !reflect.DeepEqual(txLocs, test.txLocs) {
			t.Errorf("DeserializeTxLoc #%d\n got: %s want: %s", i,
				spew.Sdump(txLocs), spew.Sdump(test.txLocs))
			continue
		}
	}
}

// TestBlockSerializeErrors performs negative tests against wire encode and
// decode of MsgBlock to confirm error paths work correctly.
func TestBlockSerializeErrors(t *testing.T) {
	tests := []struct {
		in       *wire.MsgBlock // Value to encode
		buf      []byte         // Serialized data
		max      int            // Max size of fixed buffer to induce errors
		writeErr error          // Expected write error
		readErr  error          // Expected read error
	}{
		// Force error in version.
		{&blockOne, blockOneBytes, 0, io.ErrShortWrite, io.EOF},
		// Force error in prev block hash.
		{&blockOne, blockOneBytes, 4, io.ErrShortWrite, io.EOF},
		// Force error in merkle root.
		{&blockOne, blockOneBytes, 36, io.ErrShortWrite, io.EOF},
		// Force error in timestamp.
		{&blockOne, blockOneBytes, 68, io.ErrShortWrite, io.EOF},
		// Force error in difficulty bits.
		{&blockOne, blockOneBytes, 72, io.ErrShortWrite, io.EOF},
		// Force error in header nonce.
		{&blockOne, blockOneBytes, 76, io.ErrShortWrite, io.EOF},
		// Force error in transaction count.
		{&blockOne, blockOneBytes, 80, io.ErrShortWrite, io.EOF},
		// Force error in transactions.
		{&blockOne, blockOneBytes, 81, io.ErrShortWrite, io.EOF},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Serialize the block.
		w := newFixedWriter(test.max)
		err := test.in.Serialize(w)
		if err != test.writeErr {
			t.Errorf("Serialize #%d wrong error got: %v, want: %v",
				i, err, test.writeErr)
			continue
		}

		// Deserialize the block.
		var block wire.MsgBlock
		r := newFixedReader(test.max, test.buf)
		err = block.Deserialize(r)
		if err != test.readErr {
			t.Errorf("Deserialize #%d wrong error got: %v, want: %v",
				i, err, test.readErr)
			continue
		}

		var txLocBlock wire.MsgBlock
		br := bytes.NewBuffer(test.buf[0:test.max])
		_, err = txLocBlock.DeserializeTxLoc(br)
		if err != test.readErr {
			t.Errorf("DeserializeTxLoc #%d wrong error got: %v, want: %v",
				i, err, test.readErr)
			continue
		}
	}
}

// TestBlockOverflowErrors  performs tests to ensure deserializing blocks which
// are intentionally crafted to use large values for the number of transactions
// are handled properly.  This could otherwise potentially be used as an attack
// vector.
func TestBlockOverflowErrors(t *testing.T) {
	// Use protocol version 70001 specifically here instead of the latest
	// protocol version because the test data is using bytes encoded with
	// that version.
	pver := uint32(70001)

	tests := []struct {
		buf  []byte // Wire encoding
		pver uint32 // Protocol version for wire encoding
		err  error  // Expected error
	}{
		// Block that claims to have ~uint64(0) transactions.
		{
			[]byte{
				0x01, 0x00, 0x00, 0x00, // Version 1
				0x6f, 0xe2, 0x8c, 0x0a, 0xb6, 0xf1, 0xb3, 0x72,
				0xc1, 0xa6, 0xa2, 0x46, 0xae, 0x63, 0xf7, 0x4f,
				0x93, 0x1e, 0x83, 0x65, 0xe1, 0x5a, 0x08, 0x9c,
				0x68, 0xd6, 0x19, 0x00, 0x00, 0x00, 0x00, 0x00, // PrevBlock
				0x98, 0x20, 0x51, 0xfd, 0x1e, 0x4b, 0xa7, 0x44,
				0xbb, 0xbe, 0x68, 0x0e, 0x1f, 0xee, 0x14, 0x67,
				0x7b, 0xa1, 0xa3, 0xc3, 0x54, 0x0b, 0xf7, 0xb1,
				0xcd, 0xb6, 0x06, 0xe8, 0x57, 0x23, 0x3e, 0x0e, // MerkleRoot
				0x61, 0xbc, 0x66, 0x49, // Timestamp
				0xff, 0xff, 0x00, 0x1d, // Bits
				0x01, 0xe3, 0x62, 0x99, // Nonce
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
				0xff, // TxnCount
			}, pver, &wire.MessageError{},
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Decode from wire format.
		var msg wire.MsgBlock
		r := bytes.NewReader(test.buf)
		err := msg.MsgDecode(r, test.pver)
		if reflect.TypeOf(err) != reflect.TypeOf(test.err) {
			t.Errorf("MsgDecode #%d wrong error got: %v, want: %v",
				i, err, reflect.TypeOf(test.err))
			continue
		}

		// Deserialize from wire format.
		r = bytes.NewReader(test.buf)
		err = msg.Deserialize(r)
		if reflect.TypeOf(err) != reflect.TypeOf(test.err) {
			t.Errorf("Deserialize #%d wrong error got: %v, want: %v",
				i, err, reflect.TypeOf(test.err))
			continue
		}

		// Deserialize with transaction location info from wire format.
		br := bytes.NewBuffer(test.buf)
		_, err = msg.DeserializeTxLoc(br)
		if reflect.TypeOf(err) != reflect.TypeOf(test.err) {
			t.Errorf("DeserializeTxLoc #%d wrong error got: %v, "+
				"want: %v", i, err, reflect.TypeOf(test.err))
			continue
		}
	}
}

// TestBlockSerializeSize performs tests to ensure the serialize size for
// various blocks is accurate.
func TestBlockSerializeSize(t *testing.T) {
	// Block with no transactions.
	noTxBlock := wire.NewMsgBlock(&blockOne.Header)

	tests := []struct {
		in   *wire.MsgBlock // Block to encode
		size int            // Expected serialized size
	}{
		// Block with no transactions.
		{noTxBlock, 81},

		// First block in the mainnet block chain.
		{&blockOne, len(blockOneBytes)},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		serializedSize := test.in.SerializeSize()
		if serializedSize != test.size {
			t.Errorf("MsgBlock.SerializeSize: #%d got: %d, want: "+
				"%d", i, serializedSize, test.size)
			continue
		}
	}
}

var blockOne = wire.MsgBlock{
	Header: wire.BlockHeader{
		Version: 1,
		PrevBlock: wire.ShaHash([wire.HashSize]byte{ // Make go vet happy.
			0x6f, 0xe2, 0x8c, 0x0a, 0xb6, 0xf1, 0xb3, 0x72,
			0xc1, 0xa6, 0xa2, 0x46, 0xae, 0x63, 0xf7, 0x4f,
			0x93, 0x1e, 0x83, 0x65, 0xe1, 0x5a, 0x08, 0x9c,
			0x68, 0xd6, 0x19, 0x00, 0x00, 0x00, 0x00, 0x00,
		}),
		MerkleRoot: wire.ShaHash([wire.HashSize]byte{ // Make go vet happy.
			0x98, 0x20, 0x51, 0xfd, 0x1e, 0x4b, 0xa7, 0x44,
			0xbb, 0xbe, 0x68, 0x0e, 0x1f, 0xee, 0x14, 0x67,
			0x7b, 0xa1, 0xa3, 0xc3, 0x54, 0x0b, 0xf7, 0xb1,
			0xcd, 0xb6, 0x06, 0xe8, 0x57, 0x23, 0x3e, 0x0e,
		}),

		Timestamp: time.Unix(0x4966bc61, 0), // 2009-01-08 20:54:25 -0600 CST
		Bits:      0x1d00ffff,               // 486604799
		Nonce:     0x9962e301,               // 2573394689
	},
	Transactions: []*wire.MsgTx{
		{
			Type:     -1,
			LockTime: 0,
			Data:     []byte("This is a transaction test."),
		},
	},
}

// Block one serialized bytes.
var blockOneBytes = []byte{
	0x01, 0x00, 0x00, 0x00, // Version 1
	0x6f, 0xe2, 0x8c, 0x0a, 0xb6, 0xf1, 0xb3, 0x72,
	0xc1, 0xa6, 0xa2, 0x46, 0xae, 0x63, 0xf7, 0x4f,
	0x93, 0x1e, 0x83, 0x65, 0xe1, 0x5a, 0x08, 0x9c,
	0x68, 0xd6, 0x19, 0x00, 0x00, 0x00, 0x00, 0x00, // PrevBlock
	0x98, 0x20, 0x51, 0xfd, 0x1e, 0x4b, 0xa7, 0x44,
	0xbb, 0xbe, 0x68, 0x0e, 0x1f, 0xee, 0x14, 0x67,
	0x7b, 0xa1, 0xa3, 0xc3, 0x54, 0x0b, 0xf7, 0xb1,
	0xcd, 0xb6, 0x06, 0xe8, 0x57, 0x23, 0x3e, 0x0e, // MerkleRoot
	0x61, 0xbc, 0x66, 0x49, // Timestamp
	0xff, 0xff, 0x00, 0x1d, // Bits
	0x01, 0xe3, 0x62, 0x99, // Nonce
	0x01,                   // TxnCount
	0xff, 0xff, 0xff, 0xff, // TxHeader type (-1)
	0x23, 0x00, 0x00, 0x00, // TxHeader size in bytes (35)
	0x00, 0x00, 0x00, 0x00, // TxHeader offset into the data block (0)
	0xff, 0xff, 0xff, 0xff, // Tx's Type field
	0x00, 0x00, 0x00, 0x00, // Tx's LockTime field
	0x54, 0x68, 0x69, 0x73, 0x20, 0x69, 0x73, 0x20,
	0x61, 0x20, 0x74, 0x72, 0x61, 0x6e, 0x73, 0x61,
	0x63, 0x74, 0x69, 0x6f, 0x6e, 0x20, 0x74, 0x65,
	0x73, 0x74, 0x2e, // The data in the tx's Data field

}

// Transaction location information for block one transactions.
var blockOneTxLocs = []wire.TxLoc{
	{TxType: -1, TxStart: 93, TxLen: 35},
}
