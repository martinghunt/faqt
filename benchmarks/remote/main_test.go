//go:build linux || darwin

package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const fakeChild = `#!/bin/sh
backend=avx2-16
case "$0" in *align-scalar*) backend=scalar;; esac
case "${GODEBUG-}" in *cpu.avx2=off*) backend=scalar;; esac
if [ -n "${FAQT_EXPECT_BACKEND-}" ] && [ "$backend" != "$FAQT_EXPECT_BACKEND" ]; then exit 9; fi
if [ -n "${FAQT_BACKEND_OUTPUT-}" ]; then printf '{"Backend":"%s"}\n' "$backend" > "$FAQT_BACKEND_OUTPUT"; fi
if [ -n "${FAQT_BENCH_OUTPUT-}" ]; then
 printf 'PAF fixture\n' > "$FAQT_BENCH_OUTPUT.paf"
 printf '{"Count":1,"Error":""}\n' > "$FAQT_BENCH_OUTPUT.json"
fi
exit 0
`

func fakeBundle(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	data := t.TempDir()
	for _, name := range []string{"align-scalar.test", "align-simd.test", "assembly-scalar.test", "assembly-simd.test", "runner"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(fakeChild), 0755); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"a.fa", "b.fa"} {
		if err := os.WriteFile(filepath.Join(data, name), []byte(">x\nACGT\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	hash, err := fileHash(filepath.Join(data, "a.fa"))
	if err != nil {
		t.Fatal(err)
	}
	paf := filepath.Join(dir, "temporary.paf")
	if err := os.WriteFile(paf, []byte("PAF fixture\n"), 0644); err != nil {
		t.Fatal(err)
	}
	pafHash, err := fileHash(paf)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(dir, "reference.json"), reference{Source: "test", Datasets: []dataset{{Name: "debug", A: "a.fa", B: "b.fa", AHash: hash, BHash: hash, PAFHash: pafHash, Count: 1}}}); err != nil {
		t.Fatal(err)
	}
	return dir, data
}
func TestRunnerSelfTestsAndBenchmarks(t *testing.T) {
	bundle, data := fakeBundle(t)
	out := filepath.Join(t.TempDir(), "output")
	var log bytes.Buffer
	if err := run(context.Background(), []string{"--data", data, "--out", out, "--runs", "2", "--minimap2", "none"}, bundle, &log); err != nil {
		t.Fatal(err, log.String())
	}
	var r result
	b, err := os.ReadFile(filepath.Join(out, "results.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatal(err)
	}
	if !r.Success || len(r.Checks) != 5 || len(r.Samples) != 4 || len(r.Summary) != 2 {
		t.Fatalf("unexpected results %+v", r)
	}
	for _, s := range r.Samples {
		if s.ReferenceMatch == nil || !*s.ReferenceMatch {
			t.Fatal("reference not checked")
		}
	}
	entries := archiveEntries(t, out+".tar.gz")
	if len(entries) == 0 {
		t.Fatal("empty archive")
	}
	for _, name := range entries {
		if strings.HasSuffix(name, ".paf") || strings.HasSuffix(name, ".fa") {
			t.Fatal("private input/output included in archive:", name)
		}
	}
}
func TestRunnerReturnsFailedDiagnostics(t *testing.T) {
	bundle, _ := fakeBundle(t)
	if err := os.WriteFile(filepath.Join(bundle, "align-simd.test"), []byte("#!/bin/sh\nexit 7\n"), 0755); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "failed")
	if err := run(context.Background(), []string{"--self-test-only", "--out", out}, bundle, io.Discard); err == nil {
		t.Fatal("expected failure")
	}
	var r result
	b, _ := os.ReadFile(filepath.Join(out, "results.json"))
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatal(err)
	}
	if r.Success || r.Error == "" || len(archiveEntries(t, out+".tar.gz")) == 0 {
		t.Fatal("failed diagnostics missing")
	}
}
func TestRunnerDetectsWrongAlignment(t *testing.T) {
	bundle, data := fakeBundle(t)
	broken := strings.Replace(fakeChild, "PAF fixture", "wrong PAF", 1)
	if err := os.WriteFile(filepath.Join(bundle, "assembly-simd.test"), []byte(broken), 0755); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "mismatch")
	err := run(context.Background(), []string{"--data", data, "--out", out, "--runs", "1", "--minimap2", "none"}, bundle, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "alignments differ") {
		t.Fatal("wrong output not rejected", err)
	}
}
func TestRunnerRejectsChangedInput(t *testing.T) {
	bundle, data := fakeBundle(t)
	if err := os.WriteFile(filepath.Join(data, "a.fa"), []byte("changed"), 0644); err != nil {
		t.Fatal(err)
	}
	err := run(context.Background(), []string{"--data", data, "--out", filepath.Join(t.TempDir(), "changed")}, bundle, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "input hash differs") {
		t.Fatal(err)
	}
}
func TestProcessTimeout(t *testing.T) {
	r := execute(context.Background(), 30*time.Millisecond, "timeout", []string{"/bin/sh", "-c", "sleep 5"}, environment(nil), t.TempDir(), "")
	if r.ExitCode == 0 || r.Error == "" || r.WallSeconds > 2 {
		t.Fatalf("timeout failed %+v", r)
	}
}
func TestSummarySumsBothOrders(t *testing.T) {
	s := summarize([]sample{{Dataset: "x", Backend: "minimap2", Processes: []processResult{{WallSeconds: 2, CPUSeconds: 1, PeakRSSBytes: 10}, {WallSeconds: 3, CPUSeconds: 2, PeakRSSBytes: 20}}}, {Dataset: "x", Backend: "minimap2", Processes: []processResult{{WallSeconds: 4, CPUSeconds: 3, PeakRSSBytes: 30}, {WallSeconds: 5, CPUSeconds: 4, PeakRSSBytes: 40}}}})
	if len(s) != 1 || s[0].WallMedian != 7 || s[0].CPUMedian != 5 || s[0].PeakRSSBytes != 40 {
		t.Fatal(s)
	}
}
func TestParseOptions(t *testing.T) {
	for _, args := range [][]string{{}, {"--runs", "0", "/data"}, {"--timeout", "0s", "/data"}, {"/data", "--runs", "1"}} {
		if _, err := parseOptions(args); err == nil {
			t.Fatal("accepted", args)
		}
	}
	if o, err := parseOptions([]string{"--runs", "1", "/data"}); err != nil || o.data != "/data" || o.runs != 1 {
		t.Fatal(o, err)
	}
}
func archiveEntries(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	z, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	tr := tar.NewReader(z)
	var names []string
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, h.Name)
	}
	return names
}
