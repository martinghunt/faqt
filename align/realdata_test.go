package align

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/martinghunt/faqt/seqio"
)

// Opt-in because the user's original genomes are not bundled with the library.
// This exercises the complete assembly stack, rather than a cropped sequence.
func TestSeqhikerDebugPair(t *testing.T) { testSeqhikerDebugPair(t, DefaultOptions()) }
func TestSeqhikerDebugPairAdaptive(t *testing.T) {
	o := DefaultOptions()
	o.EndExtension = EndExtensionAdaptive
	testSeqhikerDebugPair(t, o)
}
func testSeqhikerDebugPair(t *testing.T, o Options) {
	dir := os.Getenv("FAQT_SEQHIKER_DEBUG")
	if dir == "" {
		t.Skip("set FAQT_SEQHIKER_DEBUG to the directory containing the original debug FASTA files")
	}
	var gs []*Genome
	for _, name := range []string{"31585_6#49.fasta", "barcode10_ICESp23_spliced.fasta"} {
		reader, e := seqio.OpenPath(filepath.Join(dir, name))
		if e != nil {
			t.Fatal(e)
		}
		g, e := ReadGenome(reader, SketchOptions{})
		if c, ok := reader.(io.Closer); ok {
			if ce := c.Close(); e == nil {
				e = ce
			}
		}
		if e != nil {
			t.Fatal(e)
		}
		gs = append(gs, g)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	start := time.Now()
	ms, e := Compare(ctx, gs[0], gs[1], o)
	if e != nil {
		t.Fatal(e)
	}
	t.Logf("%d matches in %s", len(ms), time.Since(start))
	restored := false
	for _, m := range ms {
		replay(t, gs[0], gs[1], m, o)
		if gs[0].ContigName(m.AContig) == ".31585_6_49.4" && m.Reverse && m.A.Start <= 66800 && m.A.End >= 69700 && m.Identity() >= 0.95 {
			restored = true
		}
	}
	if !restored {
		t.Fatal("reported comparison gap has not been recovered")
	}
	swapped, e := Compare(ctx, gs[1], gs[0], o)
	if e != nil {
		t.Fatal(e)
	}
	for i := range swapped {
		swapped[i] = swapped[i].Swap()
	}
	SortMatches(swapped)
	if !reflect.DeepEqual(ms, swapped) {
		t.Fatal("real-data input swapping changed match paths")
	}
}

func TestSeqhikerDebugPairNear(t *testing.T) {
	o := DefaultOptions()
	o.EndExtension = EndExtensionAdaptive
	o.Redundancy = RedundancyNear
	testSeqhikerDebugPair(t, o)
}
