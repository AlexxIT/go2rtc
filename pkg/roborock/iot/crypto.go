package iot

import (
	"crypto/aes"
	"crypto/md5"
	"encoding/binary"
	"errors"
	"hash/crc32"
)

// key - convert timestamp to key
func (c *Codec) key(timestamp uint32) []byte {
	const salt = "TXdfu$jyZ#TZHsg4"
	key := md5.Sum([]byte(encodeTimestamp(timestamp) + c.devKey + salt))
	return key[:]
}

func (c *Codec) Decrypt(cipherText []byte) ([]byte, error) {
	if len(cipherText) < 32 || string(cipherText[:3]) != "1.0" {
		return nil, errors.New("wrong message prefix")
	}

	i := len(cipherText) - 4
	if binary.BigEndian.Uint32(cipherText[i:]) != crc32.ChecksumIEEE(cipherText[:i]) {
		return nil, errors.New("wrong message checksum")
	}

	if proto := binary.BigEndian.Uint16(cipherText[15:]); proto != 102 {
		return nil, nil
	}

	timestamp := binary.BigEndian.Uint32(cipherText[11:])
	return decryptECB(cipherText[19:i], c.key(timestamp)), nil
}

func (c *Codec) Encrypt(plainText []byte, seq, random, timestamp uint32) []byte {
	const proto = 101

	cipherText := encryptECB(plainText, c.key(timestamp))

	size := uint16(len(cipherText))

	msg := make([]byte, 23+size)
	copy(msg, "1.0")
	binary.BigEndian.PutUint32(msg[3:], seq)
	binary.BigEndian.PutUint32(msg[7:], random)
	binary.BigEndian.PutUint32(msg[11:], timestamp)
	binary.BigEndian.PutUint16(msg[15:], proto)
	binary.BigEndian.PutUint16(msg[17:], size)
	copy(msg[19:], cipherText)

	crc := crc32.ChecksumIEEE(msg[:19+size])

	binary.BigEndian.PutUint32(msg[19+size:], crc)
	return msg
}

func encodeTimestamp(i uint32) string {
	const hextable = "0123456789abcdef"
	b := []byte{
		hextable[i>>8&0xF], hextable[i>>4&0xF],
		hextable[i>>16&0xF], hextable[i&0xF],
		hextable[i>>24&0xF], hextable[i>>20&0xF],
		hextable[i>>28&0xF], hextable[i>>12&0xF],
	}
	return string(b)
}

func pad(plainText []byte, blockSize int) []byte {
	b0 := byte(blockSize - len(plainText)%blockSize)
	for i := byte(0); i < b0; i++ {
		plainText = append(plainText, b0)
	}
	return plainText
}

func unpad(paddedText []byte) []byte {
	padSize := int(paddedText[len(paddedText)-1])
	return paddedText[:len(paddedText)-padSize]
}

func encryptECB(plainText, key []byte) []byte {
	block, err := aes.NewCipher(key)
	if err != nil {
		panic(err)
	}

	blockSize := block.BlockSize()
	plainText = pad(plainText, blockSize)
	cipherText := plainText

	for len(plainText) > 0 {
		block.Encrypt(plainText, plainText)
		plainText = plainText[blockSize:]
	}

	return cipherText
}

func decryptECB(cipherText, key []byte) []byte {
	block, err := aes.NewCipher(key)
	if err != nil {
		panic(err)
	}

	blockSize := block.BlockSize()
	paddedText := cipherText

	for len(cipherText) > 0 {
		block.Decrypt(cipherText, cipherText)
		cipherText = cipherText[blockSize:]
	}

	return unpad(paddedText)
}
