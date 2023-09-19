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

// Patch - change key/value pair in YAML file without break formatting
func Patch(src []byte, key string, value any, path ...string) ([]byte, error) {
	nodeParent, err := FindParent(src, path...)
	if err != nil {
		return nil, err
	}

	var dst []byte

	if nodeParent != nil {
		dst, err = AddOrReplace(src, key, value, nodeParent)
	} else {
		dst, err = AddToEnd(src, key, value, path...)
	}

	if err = yaml.Unmarshal(dst, map[string]any{}); err != nil {
		return nil, err
	}

	return dst, nil
}

// FindParent - return YAML Node from path of keys (tree)
func FindParent(src []byte, path ...string) (*yaml.Node, error) {
	if len(src) == 0 {
		return nil, nil
	}

	var root yaml.Node
	if err := yaml.Unmarshal(src, &root); err != nil {
		return nil, err
	}

	if root.Content == nil {
		return nil, nil
	}

	parent := root.Content[0] // yaml.DocumentNode
	for _, name := range path {
		if parent == nil {
			break
		}
		_, parent = FindChild(parent, name)
	}
	return parent, nil
}

// FindChild - search and return YAML key/value pair for current Node
func FindChild(node *yaml.Node, name string) (key, value *yaml.Node) {
	for i, child := range node.Content {
		if child.Value != name {
			continue
		}
		return child, node.Content[i+1]
	}

	return nil, nil
}

func FirstChild(node *yaml.Node) *yaml.Node {
	if node.Content == nil {
		return node
	}
	return node.Content[0]
}

func LastChild(node *yaml.Node) *yaml.Node {
	if node.Content == nil {
		return node
	}
	return LastChild(node.Content[len(node.Content)-1])
}

func AddOrReplace(src []byte, key string, value any, nodeParent *yaml.Node) ([]byte, error) {
	v := map[string]any{key: value}
	put, err := Encode(v, 2)
	if err != nil {
		return nil, err
	}

	if nodeKey, nodeValue := FindChild(nodeParent, key); nodeKey != nil {
		put = AddIndent(put, nodeKey.Column-1)

		i0 := LineOffset(src, nodeKey.Line)
		i1 := LineOffset(src, LastChild(nodeValue).Line+1)

		if i1 < 0 { // no new line on the end of file
			if value != nil {
				return append(src[:i0], put...), nil
			}
			return src[:i0], nil
		}

		dst := make([]byte, 0, len(src)+len(put))
		dst = append(dst, src[:i0]...)
		if value != nil {
			dst = append(dst, put...)
		}
		return append(dst, src[i1:]...), nil
	}

	put = AddIndent(put, FirstChild(nodeParent).Column-1)

	i := LineOffset(src, LastChild(nodeParent).Line+1)

	if i < 0 { // no new line on the end of file
		src = append(src, '\n')
		if value != nil {
			src = append(src, put...)
		}
		return src, nil
	}

	dst := make([]byte, 0, len(src)+len(put))
	dst = append(dst, src[:i]...)
	if value != nil {
		dst = append(dst, put...)
	}
	return append(dst, src[i:]...), nil
}

func AddToEnd(src []byte, key string, value any, path ...string) ([]byte, error) {
	if len(path) > 1 || value == nil {
		return nil, errors.New("config: path not exist")
	}

	v := map[string]map[string]any{
		path[0]: {key: value},
	}
	put, err := Encode(v, 2)
	if err != nil {
		return nil, err
	}

	dst := make([]byte, 0, len(src)+len(put)+10)
	dst = append(dst, src...)
	if l := len(src); l > 0 && src[l-1] != '\n' {
		dst = append(dst, '\n')
	}
	return append(dst, put...), nil
}

func AddPrefix(src, pre []byte) (dst []byte) {
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

func AddIndent(src []byte, indent int) (dst []byte) {
	pre := make([]byte, indent)
	for i := 0; i < indent; i++ {
		pre[i] = ' '
	}
	return AddPrefix(src, pre)
}

func LineOffset(b []byte, line int) (offset int) {
	for l := 1; ; l++ {
		if l == line {
			return offset
		}

		i := bytes.IndexByte(b[offset:], '\n') + 1
		if i == 0 {
			break
		}
		offset += i
	}
	return -1
}
