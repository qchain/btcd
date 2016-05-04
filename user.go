package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"bufio"
	"errors"
	"crypto/aes"
	"crypto/sha256"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"github.com/qchain/btcd/btcec"
)

// TODO: Write documentation on User.
type User struct {
	Username  string
	Password []byte
	Key     []byte
}

func (u *User) Serialize(w io.Writer) error {
	// Serialize Username
	err := binary.Write(w, binary.BigEndian, u.Username)
	if err != nil {
		fmt.Println("Failed to serialize User:", err)
		return err
	}
	// Serialize Password
	err = binary.Write(w, binary.BigEndian, byte(len(u.Password)))
	if err != nil {
		fmt.Println("Failed to serialize User:", err)
		return err
	}
	err = binary.Write(w, binary.BigEndian, []byte(u.Password))
	if err != nil {
		fmt.Println("Failed to serialize User:", err)
		return err
	}
	// Serialize Key
	err = binary.Write(w, binary.BigEndian, u.Key)
	if err != nil {
		fmt.Println("Failed to serialize User:", err)
		return err
	}
	return nil
}

func (u *User) Deserialize(r io.Reader) error {
	// Deserialize Username
	err := binary.Read(r, binary.BigEndian, &u.Username)
	if err != nil {
		fmt.Println("Failed to deserialize User:", err)
		return err
	}
	// Deserialize Password
	var sizeName byte
	err = binary.Read(r, binary.BigEndian, &sizeName)
	if err != nil {
		fmt.Println("Failed to deserialize User:", err)
		return err
	}
	buf := make([]byte, sizeName)
	_, err = r.Read(buf)
	if err != nil {
		fmt.Println("Failed to deserialize User:", err)
		return err
	}
	u.Password = buf
	// Deserialize Key
	Key := make([]byte, 0, 2048)
	buf = make([]byte, 2048)
	for {
		n, err := r.Read(buf)
		Key = append(Key, buf[:n]...)
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Println("Failed to deserialize User:", err)
			return err
		}
	}
	u.Key = Key

	return nil
}

func (u *User) SetPassword(pw string) ([]byte, error) {
	h := sha256.New()
	io.WriteString(h, pw)
	return h.Sum(nil), nil
}

func (u *User) checkPassword (pw string) bool {
	h := sha256.New()
	io.WriteString(h, pw)

	for i, v := range h.Sum(nil) {
		if v != u.Password[i] {
			return false
		}
	}

	return true
}

func (u *User) newKey(pw string) ([]byte, error) {
	privKey, err := btcec.NewPrivateKey(btcec.S256())
	if err != nil {
		return nil, err
	}
	pkBytes := privKey.Serialize()
	pwb := []byte(pw)
	ciphertext, err := encrypt(pwb, pkBytes)

	return ciphertext, nil
}

func (u *User) GetKey(pw string) ([]byte, error) {
	u.checkPassword(pw)
	pwb := []byte(pw)
	pkBytes, err := decrypt(pwb, u.Key)
	if err != nil {
		return nil , err
	}
	return pkBytes, nil
}

func encrypt(key, text []byte) ([]byte, error) {
    block, err := aes.NewCipher(key)
    if err != nil {
        return nil, err
    }
    b := base64.StdEncoding.EncodeToString(text)
    ciphertext := make([]byte, aes.BlockSize+len(b))
    iv := ciphertext[:aes.BlockSize]
    if _, err := io.ReadFull(rand.Reader, iv); err != nil {
        return nil, err
    }
    cfb := cipher.NewCFBEncrypter(block, iv)
    cfb.XORKeyStream(ciphertext[aes.BlockSize:], []byte(b))
    return ciphertext, nil
}

func decrypt(key, text []byte) ([]byte, error) {
    block, err := aes.NewCipher(key)
    if err != nil {
        return nil, err
    }
    if len(text) < aes.BlockSize {
        return nil, errors.New("ciphertext too short")
    }
    iv := text[:aes.BlockSize]
    text = text[aes.BlockSize:]
    cfb := cipher.NewCFBDecrypter(block, iv)
    cfb.XORKeyStream(text, text)
    data, err := base64.StdEncoding.DecodeString(string(text))
    if err != nil {
        return nil, err
    }
    return data, nil
}


func (u *User) NewUser(username string) *User {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Enter new password : \n")
	text, err := reader.ReadString('\n')
	if err != nil {
		return nil
	}
	pwchk, err := u.SetPassword(text)

	keyb, err := u.newKey(text)
	user := &User{
		Username: username,
		Password: pwchk,
		Key: keyb,
	}
	return user
}