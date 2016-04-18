package wire

import (
	"encoding/binary"
	"fmt"
	"io"
)

type Extension string

// TODO: Write documentation on TxData.
type TxData struct {
	Version int32
	Type    Extension
	Data    []byte
}

const (
	// The Extention representing no extention. This symbolized raw data.
	// It contains one null character so not to be confused with an extension
	NONE = Extension("\x00")
)

func (tx *TxData) Serialize(w io.Writer) error {
	err := binary.Write(w, binary.BigEndian, tx.Version)
	if err != nil {
		fmt.Println("Failed to serialize TxData:", err)
		return err
	}
	err = binary.Write(w, binary.BigEndian, byte(len(tx.Type)))
	if err != nil {
		fmt.Println("Failed to serialize TxData:", err)
		return err
	}
	err = binary.Write(w, binary.BigEndian, []byte(tx.Type))
	if err != nil {
		fmt.Println("Failed to serialize TxData:", err)
		return err
	}
	err = binary.Write(w, binary.BigEndian, tx.Data)
	if err != nil {
		fmt.Println("Failed to serialize TxData:", err)
		return err
	}
	return nil
}

func (tx *TxData) Deserialize(r io.Reader) error {
	err := binary.Read(r, binary.BigEndian, &tx.Version)
	if err != nil {
		fmt.Println("Failed to deserialize TxData:", err)
		return err
	}

	var sizeType byte
	err = binary.Read(r, binary.BigEndian, &sizeType)
	if err != nil {
		fmt.Println("Failed to deserialize TxData:", err)
		return err
	}
	buf := make([]byte, sizeType)
	_, err = r.Read(buf)
	if err != nil {
		fmt.Println("Failed to deserialize TxData:", err)
		return err
	}
	tx.Type = Extension(buf)

	data := make([]byte, 0, 2048)
	buf = make([]byte, 2048)
	for {
		n, err := r.Read(buf)
		data = append(data, buf[:n]...)
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Println("Failed to deserialize TxData:", err)
			return err
		}
	}
	tx.Data = data

	return nil
}

func (tx *TxData) SerializeSize() int {
	// tx.Version + len(tx.Type) + tx.Type + tx.Data
	return 4 + 1 + len(tx.Type) + len(tx.Data)
}

func (tx *TxData) GetVersion() int32 {
	return tx.Version
}

func (tx *TxData) GetType() int32 {
	return TxTypeData
}

func NewTxData() *TxData {
	tx := &TxData{
		Version: 1,
		Type:    NONE,
		Data:    nil,
	}
	return tx
}
