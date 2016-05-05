package main

import (
	
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/qchain/btcd/btcec"
	"io"
	"io/ioutil"

)

// TODO: Write documentation on User.
type User struct {
	Version  int32
	Username string
	Password []byte
	Key      []byte
}

func (u *User) Serialize(w io.Writer) error {
	// Serialize Version
	err := binary.Write(w, binary.BigEndian, u.Version)
	if err != nil {
		fmt.Println("Failed to serialize TxFile:", err)
		return err
	}
	// Serialize Username
	err = binary.Write(w, binary.BigEndian, byte(len(u.Username)))
	if err != nil {
		fmt.Println("Failed to serialize Usernamelength:", err)
		return err
	}
	err = binary.Write(w, binary.BigEndian, []byte(u.Username))
	if err != nil {
		fmt.Println("Failed to serialize Username:", err)
		return err
	}
	// Serialize Password
	err = binary.Write(w, binary.BigEndian, byte(len(u.Password)))
	if err != nil {
		fmt.Println("Failed to serialize Passwordlength:", err)
		return err
	}
	err = binary.Write(w, binary.BigEndian, []byte(u.Password))
	if err != nil {
		fmt.Println("Failed to serialize Password:", err)
		return err
	}
	// Serialize Key
	err = binary.Write(w, binary.BigEndian, u.Key)
	if err != nil {
		fmt.Println("Failed to serialize Key:", err)
		return err
	}
	return nil
}

func (u *User) Deserialize(r io.Reader) error {
	// Deserialize Version
	err := binary.Read(r, binary.BigEndian, &u.Version)
	if err != nil {
		fmt.Println("Failed to deserialize TxFile:", err)
		return err
	}
	// Deserialize Username
	var sizeName byte
	err = binary.Read(r, binary.BigEndian, &sizeName)
	if err != nil {
		fmt.Println("Failed to deserialize Username:", err)
		return err
	}
	ubuf := make([]byte, sizeName)
	_, err = r.Read(ubuf)
	if err != nil {
		fmt.Println("Failed to deserialize Username:", err)
		return err
	}
	u.Username = string(ubuf)

	// Deserialize Password
	var sizePass byte
	err = binary.Read(r, binary.BigEndian, &sizePass)
	if err != nil {
		fmt.Println("Failed to deserialize User:", err)
		return err
	}
	pwbuf := make([]byte, sizePass)
	_, err = r.Read(pwbuf)
	if err != nil {
		fmt.Println("Failed to deserialize User:", err)
		return err
	}
	u.Password = pwbuf
	// Deserialize Key
	Key := make([]byte, 0, 2048)
	buf := make([]byte, 2048)
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
	if u.Password != nil {
		if !u.checkPassword(pw) {
			return nil, errors.New("Password didnt match.")
		}
	}
	h := sha256.New()
	io.WriteString(h, pw)
	return h.Sum(nil), nil
}

func (u *User) checkPassword(pw string) bool {
	h := sha256.New()
	io.WriteString(h, pw)

	for i, v := range h.Sum(nil) {
		if v != u.Password[i] {
			return false
		}
	}

	return true
}

func (u *User) NewKey(pw string) ([]byte, error) {
	if !u.checkPassword(pw) {
		return nil, errors.New("Password didnt match.")
	}
	privKey, err := btcec.NewPrivateKey(btcec.S256())
	if err != nil {
		return nil, err
	}
	pkBytes := privKey.Serialize()

	pwb, err := u.SetPassword(pw)
	if err != nil {
		return nil, err
	}
	ciphertext, err := encrypt(pwb, pkBytes)
	if err != nil {
		return nil, err
	}

	return ciphertext, nil
}

func (u *User) GetKey(pw string) ([]byte, error) {
	if !u.checkPassword(pw) {
		return nil, errors.New("Password didnt match.")
	}
	pwb, err := u.SetPassword(pw)
	pkBytes, err := decrypt(pwb, u.Key)
	if err != nil {
		return nil, err
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

func (u *User) Copy() *User {
	copiedUser := User{
		Version:  u.Version,
		Username: u.Username,
		Password: u.Password,
		Key:      u.Key,
	}
	return &copiedUser
}

func (u *User) SerializeSize() int {
	return len(u.Username) + len(u.Password) + len(u.Key)
}

func (u *User) Save() error {
	copiedUser := u.Copy()
	buf := bytes.NewBuffer(make([]byte, 0, copiedUser.SerializeSize()))
	_ = copiedUser.Serialize(buf)
	data := buf.Bytes()
	fmt.Printf("Bytes written: %v, %x\n", copiedUser.SerializeSize(), data)
	err := ioutil.WriteFile("users/"+u.Username, data, 0644)
	if err != nil {
		return err
	}
	return nil
}
/*
func (u *User) InitNewUser(username string) error {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Enter new password : \n")
	text, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	_, err = u.SetPassword(text)
	_, err = u.newKey(text)

	u = &User{
		Version:  1,
		Username: username,
		Password: pwchk,
		Key:      keyb,
	}
	return nil
}*/
func NewUser() *User {
	return &User{
		Version:  1,
		Username: "",
		Password: nil,
		Key:      nil,
	}
}
