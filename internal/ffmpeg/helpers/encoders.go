package helpers

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// Encoder represents an FFmpeg encoder with its attributes
type Encoder struct {
	Type          string
	FrameLevelMT  bool
	SliceLevelMT  bool
	Experimental  bool
	DrawHorizBand bool
	DirectRender  bool
	Name          string
	Description   string
}

var Encoders []Encoder

// ParseFFmpegEncoders parses the given FFmpeg encoder output and returns a slice of Encoders
func ParseFFmpegEncoders(input string) []Encoder {
	var encoders []Encoder
	scanner := bufio.NewScanner(strings.NewReader(input))

	// Define the regular expression to match encoder lines
	re := regexp.MustCompile(`^\s?([VAS])([F\.])([S\.])([X\.])([B\.])([D\.])\s+([\w-_]+)\s+(.*)$`)

	for scanner.Scan() {
		line := scanner.Text()

		// Skip lines that don't match the encoder format
		if !re.MatchString(line) {
			continue
		}

		// Extract data using the regular expression
		matches := re.FindStringSubmatch(line)
		if len(matches) == 9 {
			encoders = append(encoders, Encoder{
				Type:          string(matches[1][0]),
				FrameLevelMT:  matches[2] == "F",
				SliceLevelMT:  matches[3] == "S",
				Experimental:  matches[4] == "X",
				DrawHorizBand: matches[5] == "B",
				DirectRender:  matches[6] == "D",
				Name:          matches[7],
				Description:   matches[8],
			})
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "reading input:", err)
	}

	return encoders
}

func IsEncoderSupported(codec string) bool {
	for _, encoder := range Encoders {
		if encoder.Name == codec {
			return true
		}
	}
	return false
}
