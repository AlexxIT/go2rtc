package srtp

// https://datatracker.ietf.org/doc/html/rfc6716
// TOC Byte Configuration Parameters (MODE_BANDWIDTH_FRAME-SIZE)
const (
	SILK_NB_10    = 0
	SILK_NB_20    = 1
	SILK_NB_40    = 2
	SILK_NB_60    = 3
	SILK_MB_10    = 4
	SILK_MB_20    = 5
	SILK_MB_40    = 6
	SILK_MB_60    = 7
	SILK_WB_10    = 8
	SILK_WB_20    = 9
	SILK_WB_40    = 10
	SILK_WB_60    = 11
	HYBRID_SWB_10 = 12
	HYBRID_SWB_20 = 13
	HYBRID_FB_10  = 14
	HYBRID_FB_20  = 15
	CELT_NB_2_5   = 16
	CELT_NB_5     = 17
	CELT_NB_10    = 18
	CELT_NB_20    = 19
	CELT_WB_2_5   = 20
	CELT_WB_5     = 21
	CELT_WB_10    = 22
	CELT_WB_20    = 23
	CELT_SWB_2_5  = 24
	CELT_SWB_5    = 25
	CELT_SWB_10   = 26
	CELT_SWB_20   = 27
	CELT_FB_2_5   = 28
	CELT_FB_5     = 29
	CELT_FB_10    = 30
	CELT_FB_20    = 31
)

const (
	SAMPLE_RATE        = 16 // 16k sample rate by using opus/16000
	MAX_PAYLOAD_LENGTH = 1276
)
