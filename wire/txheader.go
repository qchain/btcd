package wire

import (
	"io"
)

type TxHeader struct {
	TxType   int32
	TxSize   uint32
	TxOffset uint32
}

const (
	//  TxType + TxSize + TxOffset. The size of the header is in bytes.
	TxHeaderSize = 4 + 4 + 4
)

func readTxHeader(r io.Reader, txHeader *TxHeader) error {
	err := readElements(r, &txHeader.TxType, &txHeader.TxSize, &txHeader.TxOffset)
	if err != nil {
		return err
	}
	return nil
}

func writeTxHeader(w io.Writer, tx *MsgTx, offset uint32) (uint32, error) {
	size := tx.SerializeSize()
	err := writeElements(w, tx.Type, uint32(size), offset)
	if err != nil {
		return 0, err
	}

	return uint32(size), nil
}
