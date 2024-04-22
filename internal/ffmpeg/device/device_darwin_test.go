package device

import (
	"net/url"
	"testing"
)

func TestQueryToInput(t *testing.T) {
	// Testing when both video and audio are empty
	t.Run("Empty video and audio", func(t *testing.T) {
		query := url.Values{}
		output := queryToInput(query)
		expected := ""
		if output != expected {
			t.Errorf("Expected %q, but got %q", expected, output)
		}
	})

	// Testing when video is not empty
	t.Run("Video not empty", func(t *testing.T) {
		query := url.Values{}
		query.Set("video", "some_video")
		output := queryToInput(query)
		expected := "-f avfoundation -i \"some_video:\""
		if output != expected {
			t.Errorf("Expected %q, but got %q", expected, output)
		}
	})

	// Testing when audio is not empty
	t.Run("Audio not empty", func(t *testing.T) {
		query := url.Values{}
		query.Set("audio", "some_audio")
		output := queryToInput(query)
		expected := "-f avfoundation -i \":some_audio\""
		if output != expected {
			t.Errorf("Expected %q, but got %q", expected, output)
		}
	})

	// Testing when both video and audio are not empty
	t.Run("Both video and audio not empty", func(t *testing.T) {
		query := url.Values{}
		query.Set("video", "some_video")
		query.Set("audio", "some_audio")
		output := queryToInput(query)
		expected := "-f avfoundation -i \"some_video:some_audio\""
		if output != expected {
			t.Errorf("Expected %q, but got %q", expected, output)
		}
	})

	// Additional test cases can be added here
}
