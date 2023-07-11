package ffmpeg

import (
	"bytes"
	"fmt"
	"os/exec"
)

func TranscodeToJPEG(b []byte, height ...int) ([]byte, error) {
	cmdArgs := []string{defaults["bin"], "-hide_banner", "-i", "-", "-f", "mjpeg"}
	if len(height) > 0 {
		cmdArgs = append(cmdArgs, "-vf", fmt.Sprintf("scale=-1:%d", height[0]))
	}
	cmdArgs = append(cmdArgs, "-")
	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	cmd.Stdin = bytes.NewBuffer(b)
	return cmd.Output()
}
