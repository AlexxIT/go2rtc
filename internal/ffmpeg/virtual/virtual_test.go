package virtual

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetInput(t *testing.T) {
	s := GetInput("video")
	require.Equal(t, "-re -f lavfi -i testsrc=decimals=2:size=1920x1080", s)

	s = GetInput("video=testsrc2&size=4K")
	require.Equal(t, "-re -f lavfi -i testsrc2=size=3840x2160", s)
}

func TestGetInputTTS(t *testing.T) {
	s := GetInputTTS("text=hello world&voice=slt")
	require.Equal(t, `-re -f lavfi -i "flite=text='hello world':voiceslt"`, s)
}
