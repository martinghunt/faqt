package alignment_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"runtime/pprof"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/martinghunt/faqt/align"
	"github.com/martinghunt/faqt/seqio"
)

type Summary struct {
	PrepareSeconds, AlignSeconds, WriteSeconds float64
	Count                                      int
	AllocatedBytes                             uint64
	Error                                      string
	Options                                    string
}

func TestComparison(t *testing.T) {
	a, b, stem := os.Getenv("FAQT_BENCH_A"), os.Getenv("FAQT_BENCH_B"), os.Getenv("FAQT_BENCH_OUTPUT")
	if a == "" || b == "" || stem == "" {
		t.Skip("set FAQT_BENCH_A, FAQT_BENCH_B and FAQT_BENCH_OUTPUT")
	}
	start := time.Now()
	var gs []*align.Genome
	for _, p := range []string{a, b} {
		r, e := seqio.OpenPath(p)
		if e != nil {
			panic(e)
		}
		g, e := align.ReadGenome(r, align.SketchOptions{})
		if c, ok := r.(io.Closer); ok {
			if closeErr := c.Close(); e == nil {
				e = closeErr
			}
		}
		if e != nil {
			panic(e)
		}
		gs = append(gs, g)
	}
	o := align.DefaultOptions()
	if v := os.Getenv("FAQT_MAX_ANCHORS"); v != "" {
		n, e := strconv.Atoi(v)
		if e != nil {
			panic(e)
		}
		o.MaxAnchors = n
	}
	switch os.Getenv("FAQT_BENCH_END_EXTENSION") {
	case "", "dp":
	case "snp1":
		o.EndExtension = align.EndExtensionOneSNP
	case "adaptive":
		o.EndExtension = align.EndExtensionAdaptive
	default:
		t.Fatal("FAQT_BENCH_END_EXTENSION must be dp, snp1 or adaptive")
	}
	summary := Summary{PrepareSeconds: time.Since(start).Seconds(), Options: fmt.Sprintf("%+v", o)}
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	if p := os.Getenv("FAQT_CPU_PROFILE"); p != "" {
		f, e := os.Create(p)
		if e != nil {
			panic(e)
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			f.Close()
			t.Fatal(err)
		}
		defer f.Close()
		defer pprof.StopCPUProfile()
	}
	start = time.Now()
	timeout := 10 * time.Minute
	if value := os.Getenv("FAQT_BENCH_TIMEOUT"); value != "" {
		var err error
		timeout, err = time.ParseDuration(value)
		if err != nil || timeout <= 0 {
			t.Fatal("FAQT_BENCH_TIMEOUT must be a positive duration")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	ms, e := align.Compare(ctx, gs[0], gs[1], o)
	summary.AlignSeconds = time.Since(start).Seconds()
	runtime.ReadMemStats(&after)
	summary.AllocatedBytes = after.TotalAlloc - before.TotalAlloc
	summary.Count = len(ms)
	if e != nil {
		summary.Error = e.Error()
	}
	start = time.Now()
	f, e := os.Create(stem + ".paf")
	if e != nil {
		panic(e)
	}
	w := bufio.NewWriter(f)
	for _, m := range ms {
		strand := "+"
		ops := m.Ops
		if m.Reverse {
			strand = "-"
			ops = slices.Clone(ops)
			slices.Reverse(ops)
		}
		fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%s\t%s\t%d\t%d\t%d\t%d\t%d\t255\tcg:Z:", gs[0].ContigName(m.AContig), gs[0].ContigLength(m.AContig), m.A.Start, m.A.End, strand, gs[1].ContigName(m.BContig), gs[1].ContigLength(m.BContig), m.B.Start, m.B.End, m.Matches, m.Columns)
		for _, op := range ops {
			fmt.Fprintf(w, "%d%c", op.Length, op.Code)
		}
		fmt.Fprintln(w)
	}
	if e := w.Flush(); e != nil {
		t.Fatal(e)
	}
	if e := f.Close(); e != nil {
		t.Fatal(e)
	}
	summary.WriteSeconds = time.Since(start).Seconds()
	f, e = os.Create(stem + ".json")
	if e != nil {
		panic(e)
	}
	if e := json.NewEncoder(f).Encode(summary); e != nil {
		t.Fatal(e)
	}
	if e := f.Close(); e != nil {
		t.Fatal(e)
	}
	if summary.Error != "" {
		fmt.Fprintln(os.Stderr, summary.Error)
		t.Fatal(summary.Error)
	}
}

func TestHarnessOutput(t *testing.T) {
	dir := t.TempDir()
	sequence := "ACGATCGGATTCGATGCTAGCTAGGCTAACGTTCAGGCTAGCATCGATGCTAGCTAGCATCGGATCGATGCTAGCTAGCATCGATGCTAGGCTAACGTTCAGGCTAGCATCGATGCTAGCTAGCATCGGATCGATGCTAGCTAGCATCGATGCTAGGCTAACGTTCAGGCTAGCATCGATGCTAGCTAGCATCGGATCGATGCTAGCTAGCATCGATGC"
	for _, name := range []string{"a", "b"} {
		if err := os.WriteFile(dir+"/"+name+".fa", []byte(">"+name+"\n"+sequence+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("FAQT_BENCH_A", dir+"/a.fa")
	t.Setenv("FAQT_BENCH_B", dir+"/b.fa")
	t.Setenv("FAQT_BENCH_OUTPUT", dir+"/result")
	t.Setenv("FAQT_MAX_ANCHORS", "2000000")
	t.Setenv("FAQT_CPU_PROFILE", "")
	t.Setenv("FAQT_BENCH_TIMEOUT", "")
	t.Setenv("FAQT_BENCH_END_EXTENSION", "")
	if !t.Run("run", TestComparison) {
		return
	}
	data, err := os.ReadFile(dir + "/result.json")
	if err != nil {
		t.Fatal(err)
	}
	var result Summary
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	if result.Count < 1 || result.Error != "" || result.PrepareSeconds <= 0 || result.AlignSeconds <= 0 {
		t.Fatalf("invalid summary: %+v", result)
	}
	data, err = os.ReadFile(dir + "/result.paf")
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("missing alignment PAF")
	}
}
