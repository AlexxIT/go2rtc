package core

import (
	"math"
	"runtime"
	"testing"
)

func TestMaxCPUThreads(t *testing.T) {
	tests := []struct {
		name string
		want int
	}{
		{
			name: "ExpectPositive",
			want: int(math.Round(math.Abs(float64(runtime.NumCPU())))) - 1,
		},
		{
			name: "CompareWithGOMAXPROCS",
			want: runtime.GOMAXPROCS(0) - 1, // This may not always equal NumCPU() if GOMAXPROCS has been set to a specific value.
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MaxCPUThreads(1); got != tt.want {
				t.Errorf("NumCPU() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCompareVersions(t *testing.T) {
	type args struct {
		v1 string
		v2 string
	}
	tests := []struct {
		name string
		args args
		want int
	}{
		{
			name: "equal versions",
			args: args{v1: "1.0.0", v2: "1.0.0"},
			want: 0,
		},
		{
			name: "v1 greater than v2",
			args: args{v1: "1.0.1", v2: "1.0.0"},
			want: 1,
		},
		{
			name: "v1 less than v2",
			args: args{v1: "1.0.0", v2: "1.0.1"},
			want: -1,
		},
		{
			name: "v1 greater with pre-release",
			args: args{v1: "1.0.1-alpha", v2: "1.0.1-beta"},
			want: -1,
		},
		{
			name: "v1 less with different major",
			args: args{v1: "1.2.3", v2: "2.1.1"},
			want: -1,
		},
		{
			name: "v1 greater with different minor",
			args: args{v1: "1.3.0", v2: "1.2.9"},
			want: 1,
		},
		{
			name: "btbn-ffmpeg ebobo version format",
			args: args{v1: "n7.0-7-gd38bf5e08e-20240411", v2: "6.1.1"},
			want: 1,
		},
		{
			name: "btbn-ffmpeg ebobo version format 2",
			args: args{v1: "n7.0-7-gd38bf5e08e-20240411", v2: "7.1"},
			want: -1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CompareVersions(tt.args.v1, tt.args.v2); got != tt.want {
				t.Errorf("CompareVersions() = %v, want %v", got, tt.want)
			}
		})
	}
}
