package align

import (
	"encoding/json"
	"os"
	"runtime"
	"testing"
)

func TestSIMDBackend(t *testing.T) {
	backend := dpArchitectureBackend()
	if want := os.Getenv("FAQT_EXPECT_BACKEND"); want != "" && want != backend {
		t.Fatalf("backend %q, expected %q", backend, want)
	}
	r := struct{ GOOS, GOARCH, Backend string }{runtime.GOOS, runtime.GOARCH, backend}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(string(data))
	if path := os.Getenv("FAQT_BACKEND_OUTPUT"); path != "" {
		if err := os.WriteFile(path, append(data, '\n'), 0644); err != nil {
			t.Fatal(err)
		}
	}
}
