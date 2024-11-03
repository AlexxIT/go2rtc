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
	if child.parents == nil {
		child.parents = []*Node{n}
	} else {
		child.parents = append(child.parents, n)
	}
	child.mu.Unlock()
}

func (n *Node) RemoveParent(parent *Node) {
	n.mu.Lock()
	for i, p := range n.parents {
		if p == parent {
			n.parents = append(n.parents[:i], n.parents[i+1:]...)
			break
		}
	}
	n.mu.Unlock()
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
    n.mu.Lock()
    if parents := n.parents; parents != nil {
        parentsCopy := make([]*Node, len(parents))
        copy(parentsCopy, parents)
        n.mu.Unlock()

        for _, parent := range parentsCopy {
            parent.RemoveChild(n)
        }
    } else {
        childsCopy := make([]*Node, len(n.childs))
        copy(childsCopy, n.childs)
        n.mu.Unlock()

        for _, child := range childsCopy {
            child.RemoveParent(n)
            if len(child.parents) == 0 {
                child.Close()
            }
        }
    }

    n.mu.Lock()
    n.childs = nil
    n.parents = nil
    n.mu.Unlock()
}

func MoveNode(dst, src *Node) {
    src.mu.Lock()
    childs := make([]*Node, len(src.childs))
    copy(childs, src.childs)
    src.childs = nil
    src.mu.Unlock()

    dst.mu.Lock()
    dst.childs = childs
    dst.mu.Unlock()

    for _, child := range childs {
        child.RemoveParent(src)
        child.AppendChild(dst)
    }
}
