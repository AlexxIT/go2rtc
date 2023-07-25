package yaml

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPatch(t *testing.T) {
	b := []byte(`# prefix`)

	b, err := Patch(b, "camera1", "url1", "streams")
	require.Nil(t, err)

	require.Equal(t, `# prefix
streams:
  camera1: url1
`, string(b))

	b, err = Patch(b, "camera2", []string{"url2", "url3"}, "streams")
	require.Nil(t, err)

	require.Equal(t, `# prefix
streams:
  camera1: url1
  camera2:
    - url2
    - url3
`, string(b))

	b, err = Patch(b, "camera1", "url4", "streams")
	require.Nil(t, err)

	require.Equal(t, `# prefix
streams:
  camera1: url4
  camera2:
    - url2
    - url3
`, string(b))

	b, err = Patch(b, "camera2", "url5", "streams")
	require.Nil(t, err)

	require.Equal(t, `# prefix
streams:
  camera1: url4
  camera2: url5
`, string(b))

	b, err = Patch(b, "camera1", nil, "streams")
	require.Nil(t, err)

	require.Equal(t, `# prefix
streams:
  camera2: url5
`, string(b))
}

func TestPatchParings(t *testing.T) {
	b := []byte(`homekit:
  camera1:
    pin: 123-45-678
streams:
  camera1: url1
`)

	pairings := map[string]string{
		"client1": "public1",
		"client2": "public2",
	}

	b, err := Patch(b, "pairings", pairings, "homekit", "camera1")
	require.Nil(t, err)

	require.Equal(t, `homekit:
  camera1:
    pin: 123-45-678
    pairings:
      client1: public1
      client2: public2
streams:
  camera1: url1
`, string(b))
}
