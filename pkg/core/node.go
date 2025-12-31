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
	Codec   *Codec
	Input   HandlerFunc
	Output  HandlerFunc
	Forward HandlerFunc

	id     uint32
	childs []*Node
	parent *Node

	owner any

	mu sync.Mutex
}

func (n *Node) SetOwner(owner any) *Node {
	n.owner = owner
	return n
}

func (n *Node) GetOwner() any {
	return n.owner
}

func (n *Node) WithParent(parent *Node) *Node {
	parent.AppendChild(n)
	return n
}

func (n *Node) AppendChild(child *Node) {
	n.mu.Lock()
	n.childs = append(n.childs, child)
	n.mu.Unlock()

	child.parent = n
}

func (n *Node) RemoveChild(child *Node) {
	n.mu.Lock()
	for i, ch := range n.childs {
		if ch == child {
			n.childs = append(n.childs[:i], n.childs[i+1:]...)
			break
		}
	}
	n.mu.Unlock()
}

func (n *Node) Close() {
	if parent := n.parent; parent != nil {
		parent.RemoveChild(n)

		if len(parent.childs) == 0 {
			parent.Close()
		}
	} else {
		for _, child := range n.childs {
			// Skip closing mixers - they manage their own lifecycle
			// Mixers are closed by RemoveParent when the last parent is removed
			if _, isMixer := child.owner.(*RTPMixer); isMixer {
				continue
			}
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
		child.parent = dst
	}
}
