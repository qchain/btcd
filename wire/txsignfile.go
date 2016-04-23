package wire

import (
	"encoding/binary"
	"fmt"
	"io"

	"github.com/qchain/btcd/btcec"
)

// TODO: Write documentation on TxSignFile.
type TxSignFile struct {
	Version   int32
	Filename  string
	Signature []byte
	Data      []byte
}

func (tx *TxSignFile) Serialize(w io.Writer) error {
	// Serialize Version
	err := binary.Write(w, binary.BigEndian, tx.Version)
	if err != nil {
		fmt.Println("Failed to serialize TxSignFile:", err)
		return err
	}
	// Serialize Filename
	err = binary.Write(w, binary.BigEndian, byte(len(tx.Filename)))
	if err != nil {
		fmt.Println("Failed to serialize TxSignFile:", err)
		return err
	}
	err = binary.Write(w, binary.BigEndian, []byte(tx.Filename))
	if err != nil {
		fmt.Println("Failed to serialize TxSignFile:", err)
		return err
	}
	// Serialize sig
	err = binary.Write(w, binary.BigEndian, byte(len(tx.Signature)))
	if err != nil {
		fmt.Println("Failed to serialize TxSignFile:", err)
		return err
	}
	err = binary.Write(w, binary.BigEndian, []byte(tx.Signature))
	if err != nil {
		fmt.Println("Failed to serialize TxSignFile:", err)
		return err
	}
	// Serialize Data
	err = binary.Write(w, binary.BigEndian, tx.Data)
	if err != nil {
		fmt.Println("Failed to serialize TxSignFile:", err)
		return err
	}
	return nil
}

func (tx *TxSignFile) Deserialize(r io.Reader) error {
	// Deserialize Version
	err := binary.Read(r, binary.BigEndian, &tx.Version)
	if err != nil {
		fmt.Println("Failed to deserialize TxSignFile:", err)
		return err
	}
	// Deserialize Filename
	var sizeName byte
	err = binary.Read(r, binary.BigEndian, &sizeName)
	if err != nil {
		fmt.Println("Failed to deserialize TxSignFile:", err)
		return err
	}
	namebuf := make([]byte, sizeName)
	_, err = r.Read(namebuf)
	if err != nil {
		fmt.Println("Failed to deserialize TxSignFile:", err)
		return err
	}
	tx.Filename = string(namebuf)
	// Deserialize sig
	var sigSize byte
	err = binary.Read(r, binary.BigEndian, &sigSize)
	if err != nil {
		fmt.Println("Failed to deserialize TxSignFile:", err)
		return err
	}
	sigbuf := make([]byte, sigSize)
	_, err = r.Read(sigbuf)
	if err != nil {
		fmt.Println("Failed to deserialize TxSignFile:", err)
		return err
	}
	tx.Signature = sigbuf
	// Deserialize Data
	data := make([]byte, 0, 2048)
	buf := make([]byte, 2048)
	for {
		n, err := r.Read(buf)
		data = append(data, buf[:n]...)
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Println("Failed to deserialize TxSignFile:", err)
			return err
		}
	}
	tx.Data = data

	return nil
}

func (tx *TxSignFile) SerializeSize() int {
	// tx.Version + len(tx.Type) + tx.Filename + tx.Data
	return 4 + 1 + len(tx.Filename) + len(tx.Data)
}

func (tx *TxSignFile) GetVersion() int32 {
	return tx.Version
}

func (tx *TxSignFile) GetType() int32 {
	return TxTypeSignFile
}

func (tx *TxSignFile) GetFilename() string {
	return tx.Filename
}

func (tx *TxSignFile) GetSig() []byte {
	return tx.Signature
}

func (tx *TxSignFile) SetSig(sig *btcec.Signature) {
	data := sig.Serialize()
	tx.Signature = data
}

func NewTxSignFile() *TxSignFile {
	tx := &TxSignFile{
		Version:   1,
		Filename:  "",
		Data:      nil,
		Signature: nil,
	}
	return tx
}
