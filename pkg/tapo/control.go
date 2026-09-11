package tapo

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

type controlClient struct {
	host     string
	username string
	password string
	client   *http.Client
}

type controlLogin struct {
	ErrorCode int `json:"error_code"`
	Result    struct {
		Stok     string `json:"stok"`
		StartSeq int    `json:"start_seq"`
		Data     struct {
			Nonce         string `json:"nonce"`
			DeviceConfirm string `json:"device_confirm"`
		} `json:"data"`
	} `json:"result"`
}

func newControlClient(host, username, password string) *controlClient {
	return &controlClient{
		host:     host,
		username: username,
		password: password,
		client: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		},
	}
}

func (c *controlClient) Move(direction int) error {
	stok, seq, key, iv, hash, cnonce, err := c.login()
	if err != nil {
		return err
	}

	request := []byte(fmt.Sprintf(`{"method":"multipleRequest","params":{"requests":[{"method":"relativeMove","params":{"motor":{"movestep":{"direction":"%03d"}}}}]}}`, direction))
	return c.request(stok, seq, key, iv, hash, cnonce, request)
}

func (c *controlClient) login() (string, int, []byte, []byte, string, string, error) {
	cnonce := strings.ToUpper(core.RandString(16, 16))

	initial := fmt.Sprintf(`{"method":"login","params":{"cnonce":"%s","encrypt_type":"3","username":"%s"}}`, cnonce, c.username)
	var first controlLogin
	if err := c.post("/", initial, nil, &first); err != nil {
		return "", 0, nil, nil, "", "", err
	}
	if first.Result.Data.Nonce == "" || first.Result.Data.DeviceConfirm == "" {
		return "", 0, nil, nil, "", "", errors.New("tapo: control authentication challenge rejected")
	}

	passwordHash := sha256Hex(c.password)
	keyHash := sha256Hex(cnonce + passwordHash + first.Result.Data.Nonce)
	if first.Result.Data.DeviceConfirm != keyHash+first.Result.Data.Nonce+cnonce {
		passwordHash = md5Hex(c.password)
		keyHash = sha256Hex(cnonce + passwordHash + first.Result.Data.Nonce)
		if first.Result.Data.DeviceConfirm != keyHash+first.Result.Data.Nonce+cnonce {
			return "", 0, nil, nil, "", "", errors.New("tapo: control authentication failed")
		}
	}

	digest := sha256Hex(passwordHash + cnonce + first.Result.Data.Nonce)
	login := fmt.Sprintf(`{"method":"login","params":{"cnonce":"%s","digest_passwd":"%s%s%s","encrypt_type":"3","username":"%s"}}`, cnonce, digest, cnonce, first.Result.Data.Nonce, c.username)
	var second controlLogin
	if err := c.post("/", login, nil, &second); err != nil {
		return "", 0, nil, nil, "", "", err
	}
	if second.ErrorCode != 0 || second.Result.Stok == "" {
		return "", 0, nil, nil, "", "", errors.New("tapo: control login failed")
	}

	key := sha256Bytes("lsk" + cnonce + first.Result.Data.Nonce + keyHash)
	iv := sha256Bytes("ivb" + cnonce + first.Result.Data.Nonce + keyHash)
	return second.Result.Stok, second.Result.StartSeq, key[:aes.BlockSize], iv[:aes.BlockSize], passwordHash, cnonce, nil
}

func (c *controlClient) request(stok string, seq int, key, iv []byte, passwordHash, cnonce string, request []byte) error {
	payload, err := encrypt(request, key, iv)
	if err != nil {
		return err
	}
	body := fmt.Sprintf(`{"method":"securePassthrough","params":{"request":"%s"}}`, base64.StdEncoding.EncodeToString(payload))
	tag := sha256Hex(sha256Hex(passwordHash+cnonce) + body + fmt.Sprint(seq))

	var response struct {
		ErrorCode int `json:"error_code"`
		Result    struct {
			Response string `json:"response"`
		} `json:"result"`
	}
	if err = c.post("/stok="+stok+"/ds", body, map[string]string{"Seq": fmt.Sprint(seq), "Tapo_tag": tag}, &response); err != nil {
		return err
	}
	if response.ErrorCode != 0 {
		return fmt.Errorf("tapo: control request failed: %d", response.ErrorCode)
	}
	if response.Result.Response == "" {
		return errors.New("tapo: control response missing")
	}
	return nil
}

func (c *controlClient) post(path, body string, headers map[string]string, value any) error {
	req, err := http.NewRequest(http.MethodPost, "https://"+c.host+path, strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	req.Header.Set("requestByApp", "true")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	res, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusUnauthorized {
		return fmt.Errorf("tapo: control request: %s", res.Status)
	}
	b, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	if err = json.NewDecoder(bytes.NewReader(b)).Decode(value); err != nil {
		return err
	}
	return nil
}

func encrypt(plaintext, key, iv []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	padding := aes.BlockSize - len(plaintext)%aes.BlockSize
	plaintext = append(plaintext, bytes.Repeat([]byte{byte(padding)}, padding)...)
	ciphertext := make([]byte, len(plaintext))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ciphertext, plaintext)
	return ciphertext, nil
}

func sha256Bytes(value string) []byte {
	sum := sha256.Sum256([]byte(value))
	return sum[:]
}

func sha256Hex(value string) string {
	return strings.ToUpper(hex.EncodeToString(sha256Bytes(value)))
}

func md5Hex(value string) string {
	sum := md5.Sum([]byte(value))
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}
