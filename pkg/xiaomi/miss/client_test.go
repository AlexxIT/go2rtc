package miss

import "testing"

func TestVideoQuality(t *testing.T) {
	tests := []struct {
		name    string
		model   string
		quality string
		want    string
	}{
		{name: "default HD", model: "unknown.camera", quality: "hd", want: "2"},
		{name: "empty defaults to HD", model: "unknown.camera", want: "2"},
		{name: "C200 HD", model: ModelC200, quality: "hd", want: "3"},
		{name: "C300 HD", model: ModelC300, quality: "hd", want: "3"},
		{name: "HLC8 HD", model: ModelHLC8, quality: "hd", want: "3"},
		{name: "Mod11 HD", model: ModelMod11, quality: "hd", want: "3"},
		{name: "SD", model: ModelHLC8, quality: "sd", want: "1"},
		{name: "auto", model: ModelHLC8, quality: "auto", want: "0"},
		{name: "explicit quality", model: ModelHLC8, quality: "4", want: "4"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := videoQuality(tt.model, tt.quality); got != tt.want {
				t.Errorf("videoQuality(%q, %q) = %q, want %q", tt.model, tt.quality, got, tt.want)
			}
		})
	}
}
