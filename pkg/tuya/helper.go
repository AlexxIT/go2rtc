package tuya

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

func FormToJSON(content any) string {
	if content == nil {
		return "{}"
	}

	jsonBytes, err := json.Marshal(content)
	if err != nil {
		return "{}"
	}

	return string(jsonBytes)
}

func ToBase64(tokenInfo *TokenInfo) (string, error) {
	jsonData, err := json.Marshal(tokenInfo)
	if err != nil {
		return "", fmt.Errorf("error marshalling token: %v", err)
	}

	encoded := base64.URLEncoding.EncodeToString(jsonData)

	return encoded, nil
}

func FromBase64(encodedTokenInfo string) (*TokenInfo, error) {
	jsonData, err := base64.URLEncoding.DecodeString(encodedTokenInfo)
	if err != nil {
		return nil, fmt.Errorf("error decoding token: %v", err)
	}

	var tokenInfo TokenInfo
	err = json.Unmarshal(jsonData, &tokenInfo)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling token: %v", err)
	}

	return &tokenInfo, nil
}

func ParseTokenInfo(tokenInfoOrString any) (*TokenInfo, error) {
	var tokenInfo *TokenInfo
	var err error

	switch v := tokenInfoOrString.(type) {
	case string:
		tokenInfo, err = FromBase64(v)
		if err != nil {
			return nil, fmt.Errorf("failed to decode base64 token: %w", err)
		}
	case *TokenInfo:
		tokenInfo = v
	case TokenInfo:
		copyOfV := v
		tokenInfo = &copyOfV
	default:
		return nil, fmt.Errorf("invalid type: %T", v)
	}

	if tokenInfo == nil {
		return nil, fmt.Errorf("token info is nil")
	}

	return tokenInfo, nil
}
