package camera

// WebRTCSolicitOfferRequest is written by the controller (UUID 8053)
type WebRTCSolicitOfferRequest struct {
	Options WebRTCOfferOptions `tlv8:"1"`
}

// WebRTCOfferOptions holds solicit-offer options
type WebRTCOfferOptions struct {
	SFrameEnabled bool `tlv8:"1"`
}

// WebRTCSolicitOfferResponse is returned after solicit-offer
type WebRTCSolicitOfferResponse struct {
	SessionIdentifier   string               `tlv8:"1"`
	SDPOffer            string               `tlv8:"2"`
	AdditionalCandidates []WebRTCICECandidate `tlv8:"3"`
	Status              byte                 `tlv8:"4"`
	SFrameConfiguration SFrameKeyData        `tlv8:"5"`
}

// WebRTCICECandidate is one ICE candidate for WebRTC signalling
type WebRTCICECandidate struct {
	Candidate     string `tlv8:"1"`
	SDPMid        string `tlv8:"2"`
	SDPMLineIndex uint16 `tlv8:"3"`
}

// SFrameKeyData is end-to-end media encryption key material
type SFrameKeyData struct {
	Key string `tlv8:"1"`
	KID uint64 `tlv8:"2"`
}

// WebRTCProvideAnswerRequest is written by the controller (UUID 8054)
type WebRTCProvideAnswerRequest struct {
	SessionIdentifier    string               `tlv8:"1"`
	SDPAnswer            string               `tlv8:"2"`
	AdditionalCandidates []WebRTCICECandidate `tlv8:"3"`
}

// WebRTCProvideAnswerResponse is returned after provide-answer
type WebRTCProvideAnswerResponse struct {
	SessionIdentifier string `tlv8:"1"`
	Status            byte   `tlv8:"2"`
}

// WebRTCStreamingControlRequest ends a WebRTC session (UUID 8056)
type WebRTCStreamingControlRequest struct {
	SessionIdentifier string `tlv8:"1"`
	Command           byte   `tlv8:"2"`
}

// WebRTCStreamingControlResponse is returned after streaming-control
type WebRTCStreamingControlResponse struct {
	SessionIdentifier string `tlv8:"1"`
	Status            byte   `tlv8:"2"`
}

// WebRTCReofferRequest renegotiates an existing session (UUID 8058)
type WebRTCReofferRequest struct {
	SessionIdentifier string             `tlv8:"1"`
	SDPOffer          string             `tlv8:"2"`
	Options           WebRTCOfferOptions `tlv8:"3"`
}

// WebRTCReofferResponse is returned after reoffer
type WebRTCReofferResponse struct {
	SessionIdentifier   string        `tlv8:"1"`
	SDPAnswer           string        `tlv8:"2"`
	Status              byte          `tlv8:"3"`
	SFrameConfiguration SFrameKeyData `tlv8:"4"`
}

// WebRTCUpdateSessionRequest updates SFrame keys (UUID 805C)
type WebRTCUpdateSessionRequest struct {
	SessionIdentifier   string          `tlv8:"1"`
	ReceiveKeysToAdd    []SFrameKeyData `tlv8:"2"`
	ReceiveKIDsToRemove []SFrameKID     `tlv8:"3"`
}

// SFrameKID identifies an SFrame key to remove
type SFrameKID struct {
	KID uint64 `tlv8:"1"`
}

// WebRTCUpdateSessionResponse is returned after update-session
type WebRTCUpdateSessionResponse struct {
	SessionIdentifier string `tlv8:"1"`
	Status            byte   `tlv8:"2"`
}

// RTPStreamingControlRequest controls multi-tier RTP streams (UUID 8045)
type RTPStreamingControlRequest struct {
	SessionIdentifier string `tlv8:"1"`
	Command           byte   `tlv8:"2"`
	VideoTier         uint32 `tlv8:"3"`
	VideoSSRC         uint32 `tlv8:"4"`
	AudioTier         uint32 `tlv8:"5"`
	AudioSSRC         uint32 `tlv8:"6"`
}

// RTPStreamingControlResponse is returned after RTP streaming control
type RTPStreamingControlResponse struct {
	SessionIdentifier string `tlv8:"1"`
	Status            byte   `tlv8:"2"`
}
