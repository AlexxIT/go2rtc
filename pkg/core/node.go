package core

import (
	"sync"

	"github.com/pion/rtp"
)

//type Packet struct {
//	Payload     []byte
//	Timestamp   uint32 // PTS if DTS == 0 else DTS
//	Composition uint32 // CTS = PTS-DTS (for support B-frames)
//	Sequence    uint16
//}

type Packet = rtp.Packet

// HandlerFunc - process input packets (just like http.HandlerFunc)
type HandlerFunc func(packet *Packet)

// Filter - a decorator for any HandlerFunc
type Filter func(handler HandlerFunc) HandlerFunc

// Node - Receiver or Sender or Filter (transform)
type Node struct {
	Codec  *Codec
	Input  HandlerFunc
	Output HandlerFunc

	id      uint32
	childs  []*Node
	parents []*Node

	mu sync.Mutex
}

func (n *Node) WithParent(parent *Node) *Node {
	parent.AppendChild(n)
	return n
}

func (n *Node) AppendChild(child *Node) {
	n.mu.Lock()
	n.childs = append(n.childs, child)
	n.mu.Unlock()

	child.mu.Lock()
	child.parents = append(child.parents, n)
	child.mu.Unlock()
}

func (n *Node) RemoveChild(child *Node) {
	n.mu.Lock()
	if i := Index(n.childs, child); i != -1 {
		n.childs = append(n.childs[:i], n.childs[i+1:]...)
	}
	n.mu.Unlock()

	child.mu.Lock()
	if i := Index(child.parents, n); i != -1 {
		child.parents = append(child.parents[:i], child.parents[i+1:]...)
	}
	child.mu.Unlock()
}

func (n *Node) Close() {
	n.mu.Lock()
	if n.parents != nil && len(n.parents) > 0 {
		parents := n.parents
		n.mu.Unlock()

		for _, parent := range parents {
			parent.RemoveChild(n)
		}
	} else {
		childs := n.childs
		n.childs = nil
		n.mu.Unlock()

		for _, child := range childs {
			child.Close()
		}
	}
}

func MoveNode(dst, src *Node) {
	src.mu.Lock()
	childs := src.childs
	src.childs = nil
	src.mu.Unlock()

	dst.mu.Lock()
	dst.childs = childs
	dst.mu.Unlock()

	for _, child := range childs {
		child.mu.Lock()
		if i := Index(child.parents, dst); i != -1 {
			child.parents = append(child.parents[:i], child.parents[i+1:]...)
		}
		child.parents = append(child.parents, dst)
		child.mu.Unlock()
	}
}
