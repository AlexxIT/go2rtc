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

	id       uint32
	childs   []*Node
	parent   *Node
	keyframe func()

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
	child.parent = n
	child.mu.Unlock()
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
		for _, childs := range n.childs {
			childs.Close()
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
		child.parent = dst
		child.mu.Unlock()
	}
}

// OnKeyframe sets the handler that asks the source of this node for a keyframe.
// The handler is called from RequestKeyframe and must not block.
func (n *Node) OnKeyframe(handler func()) {
	n.mu.Lock()
	n.keyframe = handler
	n.mu.Unlock()
}

// RequestKeyframe calls the keyframe handler of this node or of the nearest
// parent that has one. Without a handler it does nothing.
func (n *Node) RequestKeyframe() {
	for n != nil {
		n.mu.Lock()
		handler, parent := n.keyframe, n.parent
		n.mu.Unlock()

		if handler != nil {
			handler()
			return
		}
		n = parent
	}
}
