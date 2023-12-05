package shell

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestQuoteSplit(t *testing.T) {
	s := `
python "-c" 'import time
print("time", time.time())'
`
	require.Equal(t, []string{"python", "-c", "import time\nprint(\"time\", time.time())"}, QuoteSplit(s))

	s = `ffmpeg -i "video=FaceTime HD Camera" -i "DeckLink SDI (2)"`
	require.Equal(t, []string{"ffmpeg", "-i", `video=FaceTime HD Camera`, "-i", "DeckLink SDI (2)"}, QuoteSplit(s))
}
