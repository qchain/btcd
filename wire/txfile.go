package wire

import (
	"encoding/binary"
	"fmt"
	"io"
)

// TODO: Write documentation on TxFile.
type TxFile struct {
	Version  int32
	Filename string
	Data     []byte
}

func (tx *TxFile) Serialize(w io.Writer) error {
	// Serialize Version
	err := binary.Write(w, binary.BigEndian, tx.Version)
	if err != nil {
		fmt.Println("Failed to serialize TxFile:", err)
		return err
	}
	// Serialize Filename
	err = binary.Write(w, binary.BigEndian, byte(len(tx.Filename)))
	if err != nil {
		fmt.Println("Failed to serialize TxFile:", err)
		return err
	}
	err = binary.Write(w, binary.BigEndian, []byte(tx.Filename))
	if err != nil {
		fmt.Println("Failed to serialize TxFile:", err)
		return err
	}
	// Serialize Data
	err = binary.Write(w, binary.BigEndian, tx.Data)
	if err != nil {
		fmt.Println("Failed to serialize TxFile:", err)
		return err
	}
	return nil
}

func (tx *TxFile) Deserialize(r io.Reader) error {
	// Deserialize Version
	err := binary.Read(r, binary.BigEndian, &tx.Version)
	if err != nil {
		fmt.Println("Failed to deserialize TxFile:", err)
		return err
	}
	// Deserialize Filename
	var sizeName byte
	err = binary.Read(r, binary.BigEndian, &sizeName)
	if err != nil {
		fmt.Println("Failed to deserialize TxFile:", err)
		return err
	}
	buf := make([]byte, sizeName)
	_, err = r.Read(buf)
	if err != nil {
		fmt.Println("Failed to deserialize TxFile:", err)
		return err
	}
	tx.Filename = string(buf)
	// Deserialize Data
	data := make([]byte, 0, 2048)
	buf = make([]byte, 2048)
	for {
		n, err := r.Read(buf)
		data = append(data, buf[:n]...)
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Println("Failed to deserialize TxFile:", err)
			return err
		}
	}
	tx.Data = data

	return nil
}

func (tx *TxFile) SerializeSize() int {
	// tx.Version + len(tx.Type) + tx.Filename + tx.Data
	return 4 + 1 + len(tx.Filename) + len(tx.Data)
}

func (tx *TxFile) GetVersion() int32 {
	return tx.Version
}

func (tx *TxFile) GetType() int32 {
	return TxTypeFile
}

func (tx *TxFile) GetFilename() string {
	return tx.Filename
}

func NewTxFile() *TxFile {
	tx := &TxFile{
		Version:  1,
		Filename: "",
		Data:     nil,
	}
	return tx
}
