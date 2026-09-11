package tapo

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEncrypt(t *testing.T) {
	key, err := hex.DecodeString("2b7e151628aed2a6abf7158809cf4f3c")
	require.NoError(t, err)
	iv, err := hex.DecodeString("000102030405060708090a0b0c0d0e0f")
	require.NoError(t, err)

	ciphertext, err := encrypt([]byte("hello"), key, iv)
	require.NoError(t, err)
	require.Equal(t, "d8666ea8aad65cc08354b4bc43d4ff56", hex.EncodeToString(ciphertext))
}

func TestControlHashes(t *testing.T) {
	require.Equal(t, "2CF24DBA5FB0A30E26E83B2AC5B9E29E1B161E5C1FA7425E73043362938B9824", sha256Hex("hello"))
	require.Equal(t, "5D41402ABC4B2A76B9719D911017C592", md5Hex("hello"))
}
