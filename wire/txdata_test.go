package wire_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/qchain/btcd/wire"
)

func TestSerialize(t *testing.T) {
	tx := &wire.TxData{
		Version: 1,
		Type:    "OTHR",
		Data:    []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0xff},
	}

	out := []byte{0, 0, 0, 1, 4, 79, 84, 72, 82, 0, 1, 2, 3, 4, 5, 6, 255}

	buf := bytes.NewBuffer(make([]byte, 0, tx.SerializeSize()))
	err := tx.Serialize(buf)
	if err != nil {
		t.Errorf("Error during serialization: %q", err)
		return
	}

	data := buf.Bytes()
	fmt.Printf("Serialized data: %v\n", data)
	if len(data) != len(out) {
		t.Errorf("Expected len(out)=%v but got len=%v", len(out), len(data))
	}
	for i, _ := range data {
		if data[i] != out[i] {
			t.Errorf("Expected %v but got %v", out, data)
			break
		}
	}
}

func TestDeserialize(t *testing.T) {
	in := []byte{0, 0, 0, 1, 4, 79, 84, 72, 82, 0, 1, 2, 3, 4, 5, 6, 255}
	out := []byte{0, 1, 2, 3, 4, 5, 6, 255}
	tx := &wire.TxData{}

	buf := bytes.NewBuffer(in)
	err := tx.Deserialize(buf)
	if err != nil {
		t.Errorf("Error during deserialization: %q", err)
		return
	}

	fmt.Printf("TxData: %+v\n", *tx)
	if tx.Version != 1 {
		t.Errorf("Expected tx.Version=%v but got tx.Version=%v", 1, tx.Version)
	}
	if tx.Type != wire.Extension("OTHR") {
		t.Errorf("Expected tx.Type=%v but got tx.Type=%v",
			wire.Extension("OTHR"), tx.Type)
	}
	if len(tx.Data) != len(out) {
		t.Errorf("Expected len(tx.Data)=%v but got len=%v",
			len(out), len(tx.Data))
	}
	if !bytes.Equal(tx.Data, out) {
		t.Errorf("Expected %v but got %v", out, tx.Data)
	}
}

func TestSerializeDeserialize(t *testing.T) {
	txin := &wire.TxData{
		Version: 2,
		Type:    "SECRET",
		Data:    []byte{65, 77, 120, 235, 55, 44, 11, 22, 33, 0},
	}

	txout := &wire.TxData{}

	buf := bytes.NewBuffer(make([]byte, 0, txin.SerializeSize()))
	err := txin.Serialize(buf)
	if err != nil {
		t.Errorf("Error during serialization: %q", err)
		return
	}

	fmt.Printf("Serialized data: %v\n", buf.Bytes())

	err = txout.Deserialize(buf)
	if err != nil {
		t.Errorf("Error during deserialization: %q", err)
		return
	}

	fmt.Printf("TxData: %+v\n", *txout)
	if txin.Version != txout.Version {
		t.Errorf("Expected txout.Version=%v but got txout.Version=%v",
			txin.Version, txout.Version)
	}
	if txin.Type != txout.Type {
		t.Errorf("Expected txout.Type=%v but got txout.Type=%v",
			txin.Type, txout.Type)
	}
	if len(txin.Data) != len(txout.Data) {
		t.Errorf("Expected len(txout.Data)=%v but got len=%v",
			len(txin.Data), len(txout.Data))
	}
	if !bytes.Equal(txin.Data, txout.Data) {
		t.Errorf("Expected %v but got %v", txin.Data, txout.Data)
	}
}

func TestSerializeSize(t *testing.T) {
	tx := &wire.TxData{
		Version: 2,
		Type:    "SECRET",
		Data:    []byte{65, 77, 120, 235, 55, 44, 11, 22, 33, 0},
	}
	size := tx.SerializeSize()

	out := len([]byte{0, 0, 0, 2, 6, 83, 69, 67, 82, 69, 84, 65, 77, 120, 235,
		55, 44, 11, 22, 33, 0})

	if size != out {
		t.Errorf("Expected size=%v but got size=%v", out, size)
	}

}
