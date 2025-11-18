package setup

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFormatAlphaNum(t *testing.T) {
	value := int64(999)
	n := 5
	s1 := strings.ToUpper(fmt.Sprintf("%0"+strconv.Itoa(n)+"s", strconv.FormatInt(value, 36)))
	s2 := FormatInt36(value, n)
	require.Equal(t, s1, s2)
}
