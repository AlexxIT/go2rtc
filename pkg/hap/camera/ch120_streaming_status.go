package camera

const TypeStreamingStatus = "120"

type StreamingStatus struct {
	Status byte `tlv8:"1"`
}

//goland:noinspection ALL
const (
	StreamingStatusAvailable   = 0
	StreamingStatusInUse       = 1
	StreamingStatusUnavailable = 2
)
