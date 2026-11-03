package align

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

// Private captured sequences are optional and never bundled with the library.
func TestDPCapturedWorkload(t *testing.T) {
	path := os.Getenv("FAQT_DP_CAPTURED")
	if path == "" {
		t.Skip("set FAQT_DP_CAPTURED to captured DP JSONL")
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 65536), 8<<20)
	count := 0
	for scanner.Scan() {
		var s struct {
			Q, T              []byte
			Band              int
			Extend            bool
			Ops               []Op
			Score, EndQ, EndT int
			Edge              bool
		}
		if err := json.Unmarshal(scanner.Bytes(), &s); err != nil {
			t.Fatal(err)
		}
		want := dpResult{s.Ops, s.Score, s.EndQ, s.EndT, s.Edge}
		got, err := alignDPPlatform(context.Background(), s.Q, s.T, s.Band, s.Extend, DefaultOptions())
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("call %d ext=%v band=%d want=%+v got=%+v error=%v", count, s.Extend, s.Band, want, got, err)
		}
		count++
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	t.Logf("%d captured calls match exactly", count)
}
