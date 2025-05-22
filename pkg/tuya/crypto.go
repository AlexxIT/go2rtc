package tuya

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/rand"
)

// https://github.com/tuya/tuya-device-sharing-sdk/blob/main/tuya_sharing/customerapi.py
func AesGCMEncrypt(rawData string, secret string) (string, error) {
	nonce := []byte(RandomNonce(12))

	block, err := aes.NewCipher([]byte(secret))
	if err != nil {
		return "", err
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	ciphertext := aesgcm.Seal(nil, nonce, []byte(rawData), nil)
	nonceB64 := base64.StdEncoding.EncodeToString(nonce)
	ciphertextB64 := base64.StdEncoding.EncodeToString(ciphertext)

	return nonceB64 + ciphertextB64, nil
}

func AesGCMDecrypt(cipherData string, secret string) (string, error) {
	if len(cipherData) <= 16 {
		return "", fmt.Errorf("invalid ciphertext length")
	}

	nonceB64 := cipherData[:16]
	ciphertextB64 := cipherData[16:]

	nonce, err := base64.StdEncoding.DecodeString(nonceB64)
	if err != nil {
		return "", err
	}

	ciphertext, err := base64.StdEncoding.DecodeString(ciphertextB64)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher([]byte(secret))
	if err != nil {
		return "", err
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	plaintext, err := aesgcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

func SecretGenerating(rid, sid, hashKey string) string {
	message := hashKey
	mod := 16

	if sid != "" {
		sidLength := len(sid)
		length := sidLength
		if length > mod {
			length = mod
		}

		ecode := ""
		for i := 0; i < length; i++ {
			idx := int(sid[i]) % mod
			ecode += string(sid[idx])
		}
		message += "_"
		message += ecode
	}

	h := hmac.New(sha256.New, []byte(rid))
	h.Write([]byte(message))
	byteTemp := h.Sum(nil)
	secret := hex.EncodeToString(byteTemp)

	return secret[:16]
}

func RestfulSign(hashKey, queryEncdata, bodyEncdata string, data map[string]string) string {
	headers := []string{"X-appKey", "X-requestId", "X-sid", "X-time", "X-token"}
	headerSignStr := ""

	for _, item := range headers {
		val, exists := data[item]
		if exists && val != "" {
			headerSignStr += item + "=" + val + "||"
		}
	}

	signStr := ""
	if len(headerSignStr) > 2 {
		signStr = headerSignStr[:len(headerSignStr)-2]
	}

	if queryEncdata != "" {
		signStr += queryEncdata
	}
	if bodyEncdata != "" {
		signStr += bodyEncdata
	}

	h := hmac.New(sha256.New, []byte(hashKey))
	h.Write([]byte(signStr))
	return hex.EncodeToString(h.Sum(nil))
}

func RandomNonce(length int) string {
	const charset = "ABCDEFGHJKMNPQRSTWXYZabcdefhijkmnprstwxyz2345678"
	result := make([]byte, length)
	for i := range result {
		result[i] = charset[rand.Intn(len(charset))]
	}
	return string(result)
}
