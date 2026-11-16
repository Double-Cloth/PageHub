package docker

import "testing"

func TestDecodeComposePS(t *testing.T) {
	for _, data := range []string{`[{"State":"running"}]`, `{"State":"running"}`} {
		items, err := DecodeObjects([]byte(data))
		if err != nil || len(items) != 1 || items[0]["State"] != "running" {
			t.Fatalf("%#v %v", items, err)
		}
	}
}
