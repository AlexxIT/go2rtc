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

	id     uint32
	childs []*Node
	parent *Node
	closed bool // Track if node is closed

	mu sync.Mutex
}

func (n *Node) WithParent(parent *Node) *Node {
	parent.AppendChild(n)
	return n
}

func (n *Node) AppendChild(child *Node) {
	n.mu.Lock()

	// Don't add children to closed nodes
	if n.closed {
		n.mu.Unlock()
		// Parent is closed, close the orphaned child
		child.Close()
		return
	}

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

// Clean up ghost children (children that were closed but not removed)
func (n *Node) cleanGhostChildren() {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Filter out closed children
	alive := make([]*Node, 0, len(n.childs))
	for _, child := range n.childs {
		child.mu.Lock()
		isClosed := child.closed
		child.mu.Unlock()

		if !isClosed {
			alive = append(alive, child)
		}
	}

	if len(alive) != len(n.childs) {
		n.childs = alive
	}
}

func (n *Node) Close() {
	// Lock to safely read parent
	n.mu.Lock()

	// Prevent double-close
	if n.closed {
		n.mu.Unlock()
		return
	}
	n.closed = true

	parent := n.parent
	n.parent = nil // Clear parent reference
	n.mu.Unlock()

	if parent != nil {
		parent.RemoveChild(n)

		// Clean ghost children before checking
		parent.cleanGhostChildren()

		// Check if parent should close
		parent.mu.Lock()
		hasChildren := len(parent.childs) > 0
		parent.mu.Unlock()

		if !hasChildren {
			parent.Close()
		}
	} else {
		// This is a root node, close all children
		n.mu.Lock()
		children := make([]*Node, len(n.childs))
		copy(children, n.childs)
		n.childs = nil
		n.mu.Unlock()

		for _, child := range children {
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
	// Don't move to closed node
	if dst.closed {
		dst.mu.Unlock()
		// Close orphaned children
		for _, child := range childs {
			child.Close()
		}
		return
	}
	dst.childs = childs
	dst.mu.Unlock()

	for _, child := range childs {
		child.parent = dst
	}
}
