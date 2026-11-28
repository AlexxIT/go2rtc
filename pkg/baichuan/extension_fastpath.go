package baichuan

import (
	"bytes"
	"strconv"
)

func extensionAtoiSyntax(digits []byte) bool {
	if len(digits) > 0 && (digits[0] == '+' || digits[0] == '-') {
		digits = digits[1:]
	}
	if len(digits) == 0 {
		return false
	}
	for _, digit := range digits {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}

// fastExtensionAtoi handles the common extension shape; misses retain XML semantics.
func fastExtensionAtoi(body []byte) (extension, bool) {
	root, body, ok := extensionRoot(body)
	if !ok {
		return extension{}, false
	}
	var values [3]int
	var seen uint8
	for {
		body = extensionSpace(body)
		if extensionClose(body, root) {
			body = extensionSpace(body[len(root)+3:])
			if len(body) != 0 {
				return extension{}, false
			}
			return extensionPointers(values, seen), true
		}
		name, digits, rest, ok := extensionElement(body)
		if !ok {
			return extension{}, false
		}
		body = rest
		index := -1
		switch string(name) {
		case "binaryData":
			index = 0
		case "encryptLen":
			index = 1
		case "checkPos":
			index = 2
		}
		if !extensionAtoiSyntax(digits) {
			return extension{}, false
		}
		value, err := strconv.Atoi(string(digits))
		if err != nil {
			return extension{}, false
		}
		if index >= 0 {
			bit := uint8(1 << index)
			if seen&bit != 0 {
				return extension{}, false
			}
			values[index], seen = value, seen|bit
		}
	}
}

func extensionRoot(body []byte) (name, rest []byte, ok bool) {
	const declaration = `<?xml version="1.0" encoding="UTF-8" ?>`
	if bytes.HasPrefix(body, []byte(declaration)) {
		body = body[len(declaration):]
	}
	body = extensionSpace(body)
	for _, name := range []string{"body", "Extension"} {
		for _, suffix := range []string{">", ` version="1.1">`} {
			prefix := "<" + name + suffix
			if bytes.HasPrefix(body, []byte(prefix)) {
				return body[1 : len(name)+1], body[len(prefix):], true
			}
		}
	}
	return nil, nil, false
}

func extensionClose(body, name []byte) bool {
	return len(body) >= len(name)+3 && body[0] == '<' && body[1] == '/' &&
		bytes.Equal(body[2:len(name)+2], name) && body[len(name)+2] == '>'
}

func extensionElement(body []byte) (name, digits, rest []byte, ok bool) {
	if len(body) < 3 || body[0] != '<' {
		return nil, nil, nil, false
	}
	end := bytes.IndexByte(body, '>')
	if end < 2 || !extensionName(body[1:end]) {
		return nil, nil, nil, false
	}
	name, body = body[1:end], body[end+1:]
	end = bytes.IndexByte(body, '<')
	if end < 1 {
		return nil, nil, nil, false
	}
	digits, body = body[:end], body[end:]
	if len(body) < len(name)+3 || body[1] != '/' ||
		!bytes.Equal(body[2:len(name)+2], name) || body[len(name)+2] != '>' {
		return nil, nil, nil, false
	}
	return name, digits, body[len(name)+3:], true
}

func extensionSpace(body []byte) []byte {
	return bytes.TrimLeft(body, " \t\r\n")
}

func extensionName(name []byte) bool {
	for i, b := range name {
		if b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b == '_' ||
			i > 0 && (b >= '0' && b <= '9' || b == '-' || b == '.') {
			continue
		}
		return false
	}
	return len(name) != 0
}

func extensionPointers(values [3]int, seen uint8) extension {
	var result extension
	if seen&1 != 0 {
		result.BinaryData = &values[0]
	}
	if seen&2 != 0 {
		result.EncryptLen = &values[1]
	}
	if seen&4 != 0 {
		result.CheckPos = &values[2]
	}
	return result
}
