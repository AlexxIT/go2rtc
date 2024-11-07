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
    if parent == nil {
        return
    }

    n.mu.Lock()
    defer n.mu.Unlock()

    for i, p := range n.parents {
        if p == parent {
            n.parents = append(n.parents[:i], n.parents[i+1:]...)
            break
        }
    }
}

func (n *Node) RemoveChild(child *Node) {
    if child == nil {
        return
    }

    n.mu.Lock()
    defer n.mu.Unlock()

    for i, ch := range n.childs {
        if ch == child {
            n.childs = append(n.childs[:i], n.childs[i+1:]...)
            break
        }
    }
}

func (n *Node) Close() {
    n.mu.Lock()
    
    // Early return if already closed
    if n.childs == nil && n.parents == nil {
        n.mu.Unlock()
        return
    }

    // Take snapshots of current relationships
    var childsCopy []*Node
    var parentsCopy []*Node
    
    if n.childs != nil {
        childsCopy = make([]*Node, len(n.childs))
        copy(childsCopy, n.childs)
    }
    
    if n.parents != nil {
        parentsCopy = make([]*Node, len(n.parents))
        copy(parentsCopy, n.parents)
    }

    // Clear relationships immediately to prevent cycles
    n.childs = nil
    n.parents = nil
    n.mu.Unlock()

    // Handle parent cleanup
    for _, parent := range parentsCopy {
        parent.mu.Lock()
        for i, child := range parent.childs {
            if child == n {
                parent.childs = append(parent.childs[:i], parent.childs[i+1:]...)
                break
            }
        }
        parent.mu.Unlock()
    }

    // Handle child cleanup
    for _, child := range childsCopy {
        child.mu.Lock()
        var needsClose bool
        
        // Remove this node from child's parents
        for i, parent := range child.parents {
            if parent == n {
                child.parents = append(child.parents[:i], child.parents[i+1:]...)
                // If child has no more parents, it should be closed
                needsClose = len(child.parents) == 0
                break
            }
        }
        child.mu.Unlock()

        // Close child if it has no more parents
        if needsClose {
            child.Close()
        }
    }
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
