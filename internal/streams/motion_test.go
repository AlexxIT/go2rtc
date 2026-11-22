package streams

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func resetMotion() {
	motionsMu.Lock()
	motions = map[string]bool{}
	motionHandlers = nil
	motionsMu.Unlock()
}

func TestMotion(t *testing.T) {
	resetMotion()
	t.Cleanup(resetMotion)

	type change struct {
		name   string
		motion bool
	}
	var changes []change
	HandleMotionFunc(func(name string, motion bool) {
		changes = append(changes, change{name, motion})
	})

	_, ok := GetMotion("cam")
	require.False(t, ok)

	SetMotion("cam", false) // first state
	SetMotion("cam", false) // same state
	SetMotion("cam", true)
	SetMotion("cam", true) // same state
	SetMotion("cam", false)

	motion, ok := GetMotion("cam")
	require.True(t, ok)
	require.False(t, motion)
	require.Equal(t, []change{{"cam", false}, {"cam", true}, {"cam", false}}, changes)
	require.Equal(t, map[string]bool{"cam": false}, GetMotions())
}

func TestMotionRace(t *testing.T) {
	resetMotion()
	t.Cleanup(resetMotion)

	var calls int
	HandleMotionFunc(func(name string, motion bool) {
		calls++ // handlers are called under the lock
	})

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				SetMotion(name, j%2 == 0)
				_, _ = GetMotion(name)
				_ = GetMotions()
			}
		}("cam" + strconv.Itoa(i))
	}
	wg.Wait()

	require.Equal(t, 400, calls)
}

func TestApiMotion(t *testing.T) {
	resetMotion()
	t.Cleanup(resetMotion)

	streamsMu.Lock()
	streams["cam"] = NewStream(nil)
	streamsMu.Unlock()
	t.Cleanup(func() { Delete("cam") })

	do := func(method, query string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		apiMotion(w, httptest.NewRequest(method, "/api/motion"+query, nil))
		return w
	}

	require.Equal(t, http.StatusNotFound, do("POST", "?src=unknown").Code)
	require.Equal(t, http.StatusMethodNotAllowed, do("PUT", "?src=cam").Code)

	require.Equal(t, http.StatusOK, do("POST", "?src=cam").Code)
	motion, ok := GetMotion("cam")
	require.True(t, ok)
	require.True(t, motion)

	w := do("GET", "")
	require.Equal(t, http.StatusOK, w.Code)
	var states map[string]bool
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &states))
	require.Equal(t, map[string]bool{"cam": true}, states)

	require.Equal(t, http.StatusOK, do("DELETE", "?src=cam").Code)
	motion, _ = GetMotion("cam")
	require.False(t, motion)
}
