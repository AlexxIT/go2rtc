package yaml

import (
	"bytes"
	"errors"

	"gopkg.in/yaml.v3"
)

func Unmarshal(in []byte, out interface{}) (err error) {
	return yaml.Unmarshal(in, out)
}

func Encode(v any, indent int) ([]byte, error) {
	b := bytes.NewBuffer(nil)
	e := yaml.NewEncoder(b)
	e.SetIndent(indent)

	if err := e.Encode(v); err != nil {
		return nil, err
	}

	return b.Bytes(), nil
}

func Patch(in []byte, path []string, value any) ([]byte, error) {
	out, err := patch(in, path, value)
	if err != nil {
		return nil, err
	}

	// validate
	if err = yaml.Unmarshal(out, map[string]any{}); err != nil {
		return nil, err
	}

	return out, nil
}

func patch(in []byte, path []string, value any) ([]byte, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(in, &root); err != nil {
		// invalid yaml
		return nil, err
	}

	// empty in
	if len(root.Content) != 1 {
		return addToEnd(in, path, value)
	}

	// yaml is not dict
	if root.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("yaml: can't patch")
	}

	// dict items list
	nodes := root.Content[0].Content

	n := len(path) - 1

	var paste []byte
	if value != nil {
		var err error
		if paste, err = Encode(map[string]any{path[n]: value}, 2); err != nil {
			return nil, err
		}
	}

	// top-level key
	if n == 0 {
		for i := 0; i < len(nodes); i += 2 {
			if nodes[i].Value == path[0] {
				i0, i1 := nodeBounds(in, nodes[i])
				return join(in[:i0], paste, in[i1:]), nil
			}
		}
		return join(in, paste), nil
	}

	// nested key
	pKey, pVal := findNode(nodes, path[:n])
	if pKey == nil {
		return addToEnd(in, path, value)
	}

	iKey, _ := findNode(pVal.Content, path[n:])
	if iKey != nil {
		paste = addIndent(paste, iKey.Column-1)
		i0, i1 := nodeBounds(in, iKey)
		return join(in[:i0], paste, in[i1:]), nil
	}

	if pVal.Content != nil {
		paste = addIndent(paste, pVal.Column-1)
	} else {
		paste = addIndent(paste, pKey.Column+1)
	}

	_, i1 := nodeBounds(in, pKey)
	return join(in[:i1], paste, in[i1:]), nil
}

func findNode(nodes []*yaml.Node, keys []string) (key, value *yaml.Node) {
	for i, name := range keys {
		for j := 0; j < len(nodes); j += 2 {
			if nodes[j].Value == name {
				if i < len(keys)-1 {
					nodes = nodes[j+1].Content
					break
				}
				return nodes[j], nodes[j+1]
			}
		}
	}
	return nil, nil
}

func nodeBounds(in []byte, node *yaml.Node) (offset0, offset1 int) {
	// start from next line after node
	offset0 = lineOffset(in, node.Line)
	offset1 = lineOffset(in, node.Line+1)

	if offset1 < 0 {
		return offset0, len(in)
	}

	for i := offset1; i < len(in); {
		indent, length := parseLine(in[i:])
		if indent+1 != length {
			if node.Column < indent+1 {
				offset1 = i + length
			} else {
				break
			}
		}
		i += length
	}

	return
}

func addToEnd(in []byte, path []string, value any) ([]byte, error) {
	if value == nil {
		return nil, errors.New("yaml: path not exist")
	}

	var v any
	switch len(path) {
	case 1:
		v = map[string]any{path[0]: value}
	case 2:
		v = map[string]map[string]any{path[0]: {path[1]: value}}
	default:
		return nil, errors.New("yaml: path not exist")
	}

	paste, err := Encode(v, 2)
	if err != nil {
		return nil, err
	}

	return join(in, paste), nil
}

func join(items ...[]byte) []byte {
	n := len(items) - 1
	for _, b := range items {
		n += len(b)
	}

	buf := make([]byte, 0, n)
	for _, b := range items {
		if len(b) == 0 {
			continue
		}
		if n = len(buf); n > 0 && buf[n-1] != '\n' {
			buf = append(buf, '\n')
		}
		buf = append(buf, b...)
	}

	return buf
}

func addPrefix(src, pre []byte) (dst []byte) {
	for len(src) > 0 {
		dst = append(dst, pre...)
		i := bytes.IndexByte(src, '\n') + 1
		if i == 0 {
			dst = append(dst, src...)
			break
		}
		dst = append(dst, src[:i]...)
		src = src[i:]
	}

	return
}

func addIndent(in []byte, indent int) (dst []byte) {
	pre := make([]byte, indent)
	for i := 0; i < indent; i++ {
		pre[i] = ' '
	}
	return addPrefix(in, pre)
}

func lineOffset(in []byte, line int) (offset int) {
	for l := 1; ; l++ {
		if l == line {
			return offset
		}

		i := bytes.IndexByte(in[offset:], '\n') + 1
		if i == 0 {
			break
		}
		offset += i
	}
	return -1
}

func parseLine(b []byte) (indent int, length int) {
	prefix := true
	for ; length < len(b); length++ {
		switch b[length] {
		case ' ':
			if prefix {
				indent++
			}
		case '\n':
			length++
			return
		default:
			prefix = false
		}
	}
	return
}
