package camera

// BufferUploadCommandRequest starts/stops buffer upload (UUID 8013)
type BufferUploadCommandRequest struct {
	SessionID  uint64 `tlv8:"1"`
	Command    byte   `tlv8:"2"`
	Start      uint64 `tlv8:"3"`
	Stop       uint64 `tlv8:"4"`
	StopAction byte   `tlv8:"5"`
}

// BufferUploadCommandResponse is returned after buffer upload command
type BufferUploadCommandResponse struct {
	ClipID uint64 `tlv8:"1"`
}

// BufferActivityCommandRequest marks buffer activity (UUID 8017)
type BufferActivityCommandRequest struct {
	Start    uint64 `tlv8:"1"` // NTP timestamp
	Duration uint64 `tlv8:"2"` // milliseconds
	Activity byte   `tlv8:"3"`
}

// BufferEventCommandRequest queries or acknowledges events (UUID 8014)
type BufferEventCommandRequest struct {
	Command        byte   `tlv8:"1"`
	SequenceNumber uint64 `tlv8:"2"`
	Limit          uint64 `tlv8:"3"`
}

// BufferEventCommandResponse is returned after buffer event command
type BufferEventCommandResponse struct {
	Events []CameraBufferEvent `tlv8:"1"`
}

// CameraBufferEvent is one event from the camera event queue
type CameraBufferEvent struct {
	SequenceNumber  uint64                       `tlv8:"1"`
	Type            byte                         `tlv8:"2"`
	CMAFSessionStart CameraBufferEventCMAFSession `tlv8:"3"`
	CMAFSessionStop  CameraBufferEventCMAFSession `tlv8:"4"`
	Motion          CameraBufferEventMotion      `tlv8:"5"`
	CMAFError       CameraBufferEventCMAFError   `tlv8:"6"`
}

// CameraBufferEventCMAFSession identifies a CMAF session start/stop
type CameraBufferEventCMAFSession struct {
	CMAFSessionID uint64 `tlv8:"1"`
}

// CameraBufferEventMotion reports motion state
type CameraBufferEventMotion struct {
	Active bool `tlv8:"1"`
}

// CameraBufferEventCMAFError reports a CMAF ingest error
type CameraBufferEventCMAFError struct {
	CMAFSessionID uint64 `tlv8:"1"`
	CMAFError     byte   `tlv8:"2"`
}

// CameraRecordingPublishingPointValue holds the CMAF publish URL (UUID 8016)
type CameraRecordingPublishingPointValue struct {
	URL                   string        `tlv8:"1"`
	ServerCACertificates  []Certificate `tlv8:"2"`
}

// Certificate is a DER-encoded X.509 certificate
type Certificate struct {
	Certificate string `tlv8:"1"`
}

// CameraKeyValue is written to install a key (UUID 8051)
type CameraKeyValue struct {
	Key       string `tlv8:"1"`
	KeyNumber uint64 `tlv8:"2"`
}

// CameraKeyIDValue reports the current key identifier (UUID 8052)
type CameraKeyIDValue struct {
	KeyID uint64 `tlv8:"1"`
}
