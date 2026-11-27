package xiaomi

import "testing"

func TestLensSources(t *testing.T) {
	const primary = "xiaomi://user:cn@camera?did=123&model=xiaomi.camera.082ac1"
	const second = primary + "&channel=2&reuse=main"
	list := func(a, b string) map[string][]string {
		return map[string][]string{"main": {a}, "second": {b}, "other": {"rtsp://example"}}
	}
	entries := lensSources(list(primary+"&subtype=hd", second))
	owner, view := entries[primary+"&subtype=hd"], entries[second]
	if owner.pair == nil || owner.pair != view.pair || owner.secondary || !view.secondary || view.primary != primary+"&subtype=hd" {
		t.Fatal("expected one configured pair and inherited owner settings")
	}
	for _, v := range []map[string][]string{
		list(primary, primary+"&channel=2"), list(primary, second+"&subtype=sd"), list(primary, second+"&channel=0"),
		list(primary, second+"&did=456"), list(primary, primary+"&channel=2&reuse=second"),
		list(primary, "xiaomi://other:cn@camera?did=123&model=xiaomi.camera.082ac1&channel=2&reuse=main"),
		list("xiaomi://user:cn@camera?did=123&model=single", "xiaomi://user:cn@camera?did=123&model=single&channel=2&reuse=main"),
		{"main": {primary}, "alias": {primary}, "second": {second}},
		{"main": {primary}, "second": {second}, "alias": {second}},
		{"main": {primary + "&audio=1"}, "second": {second}, "alias": {second + "&audio=1"}},
	} {
		if len(lensSources(v)) != 0 {
			t.Fatalf("invalid pair accepted: %v", v)
		}
	}
}
