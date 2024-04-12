package ffmpeg

import (
	"testing"
)

func TestArgs_GetFFmpegVersion(t *testing.T) {
	type fields struct {
		Bin     string
		Global  string
		Input   string
		Codecs  []string
		Filters []string
		Output  string
		Video   int
		Audio   int
	}
	tests := []struct {
		name    string
		fields  fields
		want    string
		wantErr bool
	}{
		{
			name: "Default FFmpeg Path",
			fields: fields{
				Bin: "ffmpeg",
			},
			want:    "*",
			wantErr: false,
		},
		{
			name: "Invalid FFmpeg Path",
			fields: fields{
				Bin: "/invalid/path/to/ffmpeg",
			},
			want:    "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Args{
				Bin:     tt.fields.Bin,
				Global:  tt.fields.Global,
				Input:   tt.fields.Input,
				Codecs:  tt.fields.Codecs,
				Filters: tt.fields.Filters,
				Output:  tt.fields.Output,
				Video:   tt.fields.Video,
				Audio:   tt.fields.Audio,
			}
			got, err := a.GetFFmpegVersion()
			if (err != nil) != tt.wantErr {
				t.Errorf("Args.GetFFmpegVersion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want && tt.want != "*" {
				t.Errorf("Args.GetFFmpegVersion() = %v, want %v", got, tt.want)
			}
		})
	}
}
