//go:build darwin
// +build darwin

package device

import (
	"reflect"
	"testing"
)

func Test_DeviceInputSuffix(t *testing.T) {
	tests := []struct {
		video  string
		audio  string
		output string
	}{
		{"", "", ""},
		{"video1", "audio1", `"video1:audio1"`},
		{"video2", "", `"video2"`},
		{"", "audio2", `":audio2"`},
		{"video3", "audio3", `"video3:audio3"`},
	}

	for _, test := range tests {
		result := deviceInputSuffix(test.video, test.audio)
		if result != test.output {
			t.Errorf("deviceInputSuffix(%q, %q) = %q; want %q", test.video, test.audio, result, test.output)
		}
	}
}

func TestInitDevices(t *testing.T) {
	Bin = "ffmpeg"
	initDevices()
	if len(videos) == 0 || len(audios) == 0 {
		t.Errorf("videos or audios length is 0")
	}
	if streams[0].Name == "" || streams[0].URL == "" {
		t.Errorf("streams are not initialized correctly")
	}
}

func Test_detectInputFormats(t *testing.T) {
	type args struct {
		video string
	}
	tests := []struct {
		name    string
		args    args
		want    bool
		wantErr bool
	}{
		{
			name:    "detects input formats for main camera",
			args:    args{video: "0"},
			want:    true,
			wantErr: false,
		},
		{
			name:    "detects input formats for unpresented camera",
			args:    args{video: "99"},
			want:    false,
			wantErr: true,
		},
		{
			name:    "detects input formats for empty camera",
			args:    args{video: ""},
			want:    false,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := detectInputFormats(tt.args.video)
			if (err != nil) != tt.wantErr {
				t.Errorf("detectInputFormats() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(len(got) > 0, tt.want) {
				t.Errorf("detectInputFormats() = %v, want %v", len(got) > 0, tt.want)
			}
		})
	}
}
