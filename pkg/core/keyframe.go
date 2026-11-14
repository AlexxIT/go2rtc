package core

import "errors"

// KeyframeRequester is a producer that can ask its source for a new keyframe,
// for example after a viewer sent RTCP PLI/FIR.
// RequestKeyframe must return immediately and never block on network I/O.
type KeyframeRequester interface {
	RequestKeyframe() error
}

// KeyframeRequestSource is a consumer that reports keyframe requests from its
// viewers (RTCP PLI/FIR) for the given media.
type KeyframeRequestSource interface {
	OnKeyframeRequest(media *Media, handler func())
}

// ErrKeyframeUnsupported is returned by producers that cannot request a keyframe.
var ErrKeyframeUnsupported = errors.New("keyframe request not supported")
