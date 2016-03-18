// Copyright (c) 2013-2015 The qchain developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"encoding/binary"
	"io"
)

const (
	// TxVersion is the current latest supported transaction version.
	TxVersion = 1

	// minTxPayload is the minimum payload size for a transaction.  Note
	// that any realistically usable transaction must have at least one
	// input or output, but that is a rule enforced at a higher layer, so
	// it is intentionally not included here.
	// Version 4 bytes + Varint number of transaction inputs 1 byte + Varint
	// number of transaction outputs 1 byte + LockTime 4 bytes + min input
	// payload + min output payload.
	minTxPayload = 10
)

// List of transaction types and there numerical equivalent.
const (
	UnknownType = -1
	TxDataType  = 1
)

// All current and future transactions should adherere to this interface.
// This is to allow for simple convertions from and to specialized go structs
// and the more generic MsgTX struct.
//
// Serialize should return a byte-slice representation of the
// transaction. The internal byte format should be the same as used by the
// Deserialize method below.
//
// Deserialize will take byte-slice representation of the transaction and populate
// the underlying data struct based on the format used by Serialize above.
// The underlying struct will be overwritten with the new data. However the
// underlying struct should only be changed if error returns nil.
type TxInterface interface {
	Serialize(w io.Writer) error
	Deserialize(r io.Reader) error
	SerializeSize() int
	GetVersion() int32
}

// MsgTx implements the Message interface and represents a generic tx message.
// It is used to deliver transaction information in response to a getdata
// message (MsgGetData) for a given transaction. Type stores value assosiated
// with a certain transaction type.
type MsgTx struct {
	Type     int32
	Data     []byte
	LockTime uint32
}

// SetData sets data to the transaction message.
func (msg *MsgTx) SetData(data []byte) {
	msg.Data = data
}

//  AppendData appends data to the transaction message.
func (msg *MsgTx) AppendData(data []byte) {
	msg.Data = append(msg.Data, data...)
}

// TxSha generates the ShaHash name for the transaction.
func (msg *MsgTx) TxSha() ShaHash {
	// Serialize the transaction and calculate double sha256 on the result.
	// Ignore the error returns since the only way the encode could fail
	// is being out of memory or due to nil pointers, both of which would
	// cause a run-time panic.
	buf := bytes.NewBuffer(make([]byte, 0, msg.SerializeSize()))
	_ = msg.Serialize(buf)
	return DoubleSha256SH(buf.Bytes())
}

// Copy creates a deep copy of a transaction so that the original does not get
// modified when the copy is manipulated.
func (msg *MsgTx) Copy() *MsgTx {
	newTx := MsgTx{
		Type:     msg.Type,
		Data:     make([]byte, 0, len(msg.Data)),
		LockTime: msg.LockTime,
	}
	copy(msg.Data, newTx.Data)

	return &newTx
}

// MsgDecode decodes r using the bitcoin protocol encoding into the receiver.
// This is part of the Message interface implementation.
// See Deserialize for decoding transactions stored to disk, such as in a
// database, as opposed to decoding transactions from the wire.
func (msg *MsgTx) MsgDecode(r io.Reader, pver uint32) error {
	var buf [4]byte
	_, err := io.ReadFull(r, buf[:])
	if err != nil {
		return err
	}
	msg.Type = int32(binary.LittleEndian.Uint32(buf[:]))

	// TODO: Implement this

	return nil
}

// Deserialize decodes a transaction from r into the receiver using a format
// that is suitable for long-term storage such as a database while respecting
// the Type field in the transaction.  This function differs from MsgDecode
// in that MsgDecode decodes from the bitcoin wire protocol as it was sent
// across the network.  The wire encoding can technically differ depending on
// the protocol Type and doesn't even really need to match the format of a
// stored transaction at all.  As of the time this comment was written, the
// encoded transaction is the same in both instances, but there is a distinct
// difference and separating the two allows the API to be flexible enough to
// deal with changes.
func (msg *MsgTx) Deserialize(r io.Reader) error {
	// At the current time, there is no difference between the wire encoding
	// at protocol Type 0 and the stable long-term storage format.  As
	// a result, make use of MsgDecode.
	return msg.MsgDecode(r, 0)
}

// MsgEncode encodes the receiver to w using the bitcoin protocol encoding.
// This is part of the Message interface implementation.
// See Serialize for encoding transactions to be stored to disk, such as in a
// database, as opposed to encoding transactions for the wire.
func (msg *MsgTx) MsgEncode(w io.Writer, pver uint32) error {
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], uint32(msg.Type))
	_, err := w.Write(buf[:])
	if err != nil {
		return err
	}

	_, err = w.Write(msg.Data)
	if err != nil {
		return err
	}

	binary.LittleEndian.PutUint32(buf[:], msg.LockTime)
	_, err = w.Write(buf[:])
	if err != nil {
		return err
	}

	return nil
}

// Serialize encodes the transaction to w using a format that suitable for
// long-term storage such as a database while respecting the Type field in
// the transaction.  This function differs from MsgEncode in that MsgEncode
// encodes the transaction to the bitcoin wire protocol in order to be sent
// across the network.  The wire encoding can technically differ depending on
// the protocol Type and doesn't even really need to match the format of a
// stored transaction at all.  As of the time this comment was written, the
// encoded transaction is the same in both instances, but there is a distinct
// difference and separating the two allows the API to be flexible enough to
// deal with changes.
func (msg *MsgTx) Serialize(w io.Writer) error {
	// At the current time, there is no difference between the wire encoding
	// at protocol Type 0 and the stable long-term storage format.  As
	// a result, make use of MsgEncode.
	return msg.MsgEncode(w, 0)

}

// SerializeSize returns the number of bytes it would take to serialize the
// the transaction.
func (msg *MsgTx) SerializeSize() int {
	// Type 4 bytes + LockTime 4 bytes + Serialized varint size for the
	// number of transaction inputs and outputs.
	// n := 8 + VarIntSerializeSize(uint64(len(msg.TxIn))) +
	// 	VarIntSerializeSize(uint64(len(msg.TxOut)))

	// for _, txIn := range msg.TxIn {
	// 	n += txIn.SerializeSize()
	// }

	// for _, txOut := range msg.TxOut {
	// 	n += txOut.SerializeSize()
	// }

	// TODO: Figure out what this is suppose to do and maybe implement it or
	// remove it.

	return 0
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgTx) Command() string {
	return CmdTx
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.  This is part of the Message interface implementation.
func (msg *MsgTx) MaxPayloadLength(pver uint32) uint32 {
	return MaxBlockPayload
}

func getType(tx TxInterface) int32 {
	// TODO: Implement this
	panic("Not implemented yet")
	return -1
}

// NewMsgTx returns a new generic tx message that conforms to the Message
// interface. The lock time is set to zero to indicate the transaction is
// valid immediately as opposed to some time in future.
func NewMsgTx() *MsgTx {
	return &MsgTx{
		Type:     -1,
		Data:     make([]byte, 0),
		LockTime: uint32(0),
	}
}

// Surrounds the specific transaction with generic message transaction to
// be sent over the wire and other non-transaction specific operations.
func WrapMsgTx(tx TxInterface) *MsgTx {
	buf := bytes.NewBuffer(make([]byte, 0, tx.SerializeSize()))
	_ = tx.Serialize(buf)
	data := buf.Bytes()
	return &MsgTx{
		Type:     getType(tx),
		Data:     data,
		LockTime: uint32(0),
	}
}
