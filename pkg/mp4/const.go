package mp4

import (
	"encoding/binary"
	"github.com/deepch/vdk/format/mp4/mp4io"
	"time"
)

var matrix = [9]int32{0x10000, 0, 0, 0, 0x10000, 0, 0, 0, 0x40000000}
var time0 = time.Date(1904, time.January, 1, 0, 0, 0, 0, time.UTC)

func FTYP() []byte {
	b := make([]byte, 0x18)
	binary.BigEndian.PutUint32(b, 0x18)
	copy(b[0x04:], "ftyp")
	copy(b[0x08:], "iso5")
	copy(b[0x10:], "iso5")
	copy(b[0x14:], "avc1")
	return b
}

func MOOV() *mp4io.Movie {
	return &mp4io.Movie{
		Header: &mp4io.MovieHeader{
			PreferredRate:     1,
			PreferredVolume:   1,
			Matrix:            matrix,
			NextTrackId:       -1,
			Duration:          0,
			TimeScale:         1000,
			CreateTime:        time0,
			ModifyTime:        time0,
			PreviewTime:       time0,
			PreviewDuration:   time0,
			PosterTime:        time0,
			SelectionTime:     time0,
			SelectionDuration: time0,
			CurrentTime:       time0,
		},
		MovieExtend: &mp4io.MovieExtend{
			Tracks: []*mp4io.TrackExtend{
				{
					TrackId:               1,
					DefaultSampleDescIdx:  1,
					DefaultSampleDuration: 40,
				},
			},
		},
	}
}

func TRAK() *mp4io.Track {
	return &mp4io.Track{
		// trak > tkhd
		Header: &mp4io.TrackHeader{
			TrackId:    int32(1), // change me
			Flags:      0x0007,   // 7 ENABLED IN-MOVIE IN-PREVIEW
			Duration:   0,        // OK
			Matrix:     matrix,
			CreateTime: time0,
			ModifyTime: time0,
		},
		// trak > mdia
		Media: &mp4io.Media{
			// trak > mdia > mdhd
			Header: &mp4io.MediaHeader{
				TimeScale:  1000,
				Duration:   0,
				Language:   0x55C4,
				CreateTime: time0,
				ModifyTime: time0,
			},
			// trak > mdia > minf
			Info: &mp4io.MediaInfo{
				// trak > mdia > minf > dinf
				Data: &mp4io.DataInfo{
					Refer: &mp4io.DataRefer{
						Url: &mp4io.DataReferUrl{
							Flags: 0x000001, // self reference
						},
					},
				},
				// trak > mdia > minf > stbl
				Sample: &mp4io.SampleTable{
					SampleDesc:    &mp4io.SampleDesc{},
					TimeToSample:  &mp4io.TimeToSample{},
					SampleToChunk: &mp4io.SampleToChunk{},
					SampleSize:    &mp4io.SampleSize{},
					ChunkOffset:   &mp4io.ChunkOffset{},
				},
			},
		},
	}
}
