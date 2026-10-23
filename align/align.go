// Package align compares assembled DNA genomes in pure Go. It reports local
// homologous placements, including repeats, rather than primary read mappings.
// All coordinates are zero-based, half-open, on the input forward sequences.
package align

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"slices"
	"strings"

	"github.com/martinghunt/faqt/minimizer"
	"github.com/martinghunt/faqt/seq"
	"github.com/martinghunt/faqt/seqio"
)

// SketchOptions must be the same for both prepared genomes. Odd K avoids
// strand-ambiguous palindromic k-mers. The zero value selects K=15, W=10.
type SketchOptions struct{ K, W int }

type contig struct {
	name           string
	bases, reverse []byte
	seeds          []minimizer.Minimizer
	index          map[uint64][]seedPos
}

// Genome owns normalized copies of its sequences and reusable seed indexes.
// After preparation it is immutable and safe for concurrent comparisons.
type Genome struct {
	contigs []contig
	sketch  SketchOptions
	digest  [32]byte
}

func (g *Genome) ContigCount() int        { return len(g.contigs) }
func (g *Genome) ContigName(i int) string { return g.contigs[i].name }
func (g *Genome) ContigLength(i int) int  { return len(g.contigs[i].bases) }

// Prepare copies records; names and qualities do not participate in alignment.
// Ambiguous IUPAC bases are retained as N, break seeds, and never score as matches.
func Prepare(records []seqio.SeqRecord, sketch SketchOptions) (*Genome, error) {
	i := 0
	return prepare(func() (*seqio.SeqRecord, error) {
		if i == len(records) {
			return nil, io.EOF
		}
		r := &records[i]
		i++
		return r, nil
	}, sketch)
}

func prepare(next func() (*seqio.SeqRecord, error), sketch SketchOptions) (*Genome, error) {
	if sketch.K == 0 {
		sketch.K = 15
	}
	if sketch.W == 0 {
		sketch.W = 10
	}
	if sketch.K < 3 || sketch.K > 31 || sketch.K%2 == 0 || sketch.W < 1 || sketch.W > 255 {
		return nil, fmt.Errorf("require odd K in [3,31] and W in [1,255]")
	}
	g := &Genome{sketch: sketch}
	h := sha256.New()
	var size [8]byte
	for {
		r, err := next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if r == nil {
			return nil, fmt.Errorf("sequence reader returned a nil record")
		}
		b := make([]byte, len(r.Seq))
		for i, c := range r.Seq {
			switch c {
			case 'a', 'A':
				b[i] = 'A'
			case 'c', 'C':
				b[i] = 'C'
			case 'g', 'G':
				b[i] = 'G'
			case 't', 'T', 'u', 'U':
				b[i] = 'T'
			case 'n', 'N', 'r', 'R', 'y', 'Y', 's', 'S', 'w', 'W', 'k', 'K', 'm', 'M', 'b', 'B', 'd', 'D', 'h', 'H', 'v', 'V':
				b[i] = 'N'
			default:
				return nil, fmt.Errorf("contig %q: invalid DNA byte %q at %d", r.Name, c, i)
			}
		}
		binary.LittleEndian.PutUint64(size[:], uint64(len(b)))
		h.Write(size[:])
		h.Write(b)
		seeds := minimizer.Sketch(b, sketch.K, sketch.W)
		// Sketch may emit a tied minimizer more than once. Never multiply its hits.
		slices.SortFunc(seeds, func(a, b minimizer.Minimizer) int { return a.Pos - b.Pos })
		seeds = slices.CompactFunc(seeds, func(a, b minimizer.Minimizer) bool { return a.Pos == b.Pos })
		index := make(map[uint64][]seedPos, len(seeds))
		for _, seed := range seeds {
			index[seed.Hash] = append(index[seed.Hash], seedPos{seed.Pos, seed.Strand})
		}
		g.contigs = append(g.contigs, contig{r.Name, b, reverseComplement(b), seeds, index})
	}
	copy(g.digest[:], h.Sum(nil))
	return g, nil
}

// ReadGenome streams records from an already opened sequence reader, then
// prepares the in-memory genome required for random-access alignment.
func ReadGenome(reader seqio.Reader, sketch SketchOptions) (*Genome, error) {
	if reader == nil {
		return nil, fmt.Errorf("reader is required")
	}
	return prepare(reader.Read, sketch)
}

// EndExtensionMode selects how candidate alignments grow beyond their outermost
// exact anchors. Internal gaps between anchors always use gapped alignment.
type EndExtensionMode uint8

const (
	// EndExtensionDP uses the full banded, two-piece affine extension kernel.
	EndExtensionDP EndExtensionMode = iota
	// EndExtensionOneSNP scans without gaps, allowing at most one canonical
	// DNA substitution per end and retaining the highest-scoring prefix.
	EndExtensionOneSNP

	// EndExtensionAdaptive grows a banded end search from 64 bases when boundary
	// paths remain competitive. It is faster but heuristic: later recovery across
	// a score valley can be missed. Internal gaps retain full alignment.
	EndExtensionAdaptive
)

// Options controls sensitivity, alignment scoring, and explicit resource limits.
// Use DefaultOptions and modify fields; Compare does not fill individual zeros.
type Options struct {
	MinLength                  int
	MinIdentity                float64          // [0,1], exact A/C/G/T matches / all alignment columns
	MinChainScore              int              // minimum chaining coverage score
	MinScore                   int              // minimum final base-alignment score
	MaxGap                     int              // maximum unanchored distance on either sequence
	BandWidth                  int              // initial DP band; global gaps widen adaptively
	Extension                  int              // maximum unseeded extension on either side of a chain
	EndExtension               EndExtensionMode // zero value keeps full DP extension
	ZDrop                      int              // split a chain when an intervening alignment scores below -ZDrop
	Match, Mismatch            int
	GapOpen, GapExtend         int            // positive penalties: open + length*extend
	LongGapOpen, LongGapExtend int            // second affine cost; lower cost wins
	MaxAnchors                 int            // per contig pair/strand, before compaction; 0 means unlimited
	MaxCells                   int64          // per DP call; 0 means unlimited
	Progress                   func(Progress) // synchronous; never called concurrently by one Compare
}

type Progress struct {
	Completed, Total int
	Phase            string
}

func DefaultOptions() Options {
	return Options{MinLength: 100, MinIdentity: 0.7, MinChainScore: 40, MinScore: 40, MaxGap: 2000, BandWidth: 96, Extension: 256, ZDrop: 200, Match: 2, Mismatch: 4, GapOpen: 4, GapExtend: 2, LongGapOpen: 24, LongGapExtend: 1, MaxAnchors: 2000000, MaxCells: 16000000}
}
func (o Options) validate() error {
	if o.MinLength < 1 || math.IsNaN(o.MinIdentity) || o.MinIdentity < 0 || o.MinIdentity > 1 || o.MinChainScore < 1 || o.MinScore < 0 || o.MaxGap < 1 || o.BandWidth < 1 || o.Extension < 0 || o.EndExtension > EndExtensionAdaptive || o.ZDrop < 0 || o.Match < 1 || o.Mismatch < 1 || o.GapOpen < 1 || o.GapExtend < 1 || o.LongGapOpen < 1 || o.LongGapExtend < 1 || o.MaxAnchors < 0 || o.MaxCells < 0 {
		return fmt.Errorf("invalid alignment options; start with DefaultOptions()")
	}
	return nil
}

// LimitError means the search could not complete. Compare returns no partial
// result. Callers can raise the named limit or report an incomplete comparison.
type LimitError struct {
	Resource        string
	Required, Limit int64
}

func (e *LimitError) Error() string {
	return fmt.Sprintf("alignment %s limit exceeded: need %d, limit %d", e.Resource, e.Required, e.Limit)
}

// Op is a run of = (exact DNA match), X (substitution, including N),
// I (consumes A only), or D (consumes B only). For reverse matches, operations
// traverse A forwards and B backwards. No clipping operations are included.
type Op struct {
	Code   byte
	Length int
}

// Match describes one local alignment on a pair of original contigs.
type Match struct {
	AContig, BContig        int
	A, B                    seq.Interval
	Reverse                 bool
	Ops                     []Op
	Matches, Columns, Score int
}

func (m Match) Identity() float64 {
	if m.Columns == 0 {
		return 0
	}
	return float64(m.Matches) / float64(m.Columns)
}
func (m Match) CIGAR() string {
	var s strings.Builder
	for _, op := range m.Ops {
		fmt.Fprintf(&s, "%d%c", op.Length, op.Code)
	}
	return s.String()
}

// Swap exchanges genomes, including operation orientation and indel direction.
func (m Match) Swap() Match {
	m.AContig, m.BContig = m.BContig, m.AContig
	m.A, m.B = m.B, m.A
	m.Ops = slices.Clone(m.Ops)
	if m.Reverse {
		slices.Reverse(m.Ops)
	}
	for i := range m.Ops {
		switch m.Ops[i].Code {
		case 'I':
			m.Ops[i].Code = 'D'
		case 'D':
			m.Ops[i].Code = 'I'
		}
	}
	return m
}

// Compare searches both directions in a canonical content order, retaining
// distinct homologous placements. Input order affects only coordinate labelling.
// It is a seed-based heuristic, not exhaustive enumeration of all local paths.
// Cancellation and resource exhaustion return an error and no partial matches.
func Compare(ctx context.Context, a, b *Genome, o Options) ([]Match, error) {
	if ctx == nil || a == nil || b == nil {
		return nil, fmt.Errorf("context and both genomes are required")
	}
	if err := o.validate(); err != nil {
		return nil, err
	}
	if a.sketch != b.sketch {
		return nil, fmt.Errorf("genomes must use identical sketch options")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	swapped := slices.Compare(a.digest[:], b.digest[:]) > 0
	if swapped {
		a, b = b, a
	}
	total := len(a.contigs) * len(b.contigs) * 2
	done := 0
	var matches []Match
	for ai := range a.contigs {
		for bi := range b.contigs {
			for direction := 0; direction < 2; direction++ {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				q, t := a.contigs[ai], b.contigs[bi]
				if direction == 1 {
					q, t = t, q
				}
				found, err := compareContigs(ctx, q, t, a.sketch, o)
				if err != nil {
					return nil, fmt.Errorf("contigs %q / %q: %w", q.name, t.name, err)
				}
				for _, m := range found {
					if direction == 1 {
						m = m.Swap()
					}
					m.AContig, m.BContig = ai, bi
					matches = append(matches, m)
				}
				done++
				if o.Progress != nil {
					o.Progress(Progress{done, total, "comparing contigs"})
				}
			}
		}
	}
	// Equal-content genomes have no distinguished canonical side. Close the set
	// under swapping, including off-diagonal self-repeat placements.
	if a.digest == b.digest {
		n := len(matches)
		for i := 0; i < n; i++ {
			matches = append(matches, matches[i].Swap())
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var err error
	matches, err = deduplicateContext(ctx, matches)
	if err != nil {
		return nil, err
	}
	if swapped {
		for i := range matches {
			matches[i] = matches[i].Swap()
		}
	}
	SortMatches(matches)
	return matches, nil
}

// SortMatches provides deterministic presentation order on the caller's axes.
func SortMatches(matches []Match) { slices.SortFunc(matches, compareMatch) }
func compareMatch(a, b Match) int {
	for _, p := range [][2]int{{a.AContig, b.AContig}, {a.BContig, b.BContig}, {a.A.Start, b.A.Start}, {a.A.End, b.A.End}, {a.B.Start, b.B.Start}, {a.B.End, b.B.End}} {
		if p[0] < p[1] {
			return -1
		}
		if p[0] > p[1] {
			return 1
		}
	}
	if a.Reverse != b.Reverse {
		if !a.Reverse {
			return -1
		}
		return 1
	}
	return strings.Compare(a.CIGAR(), b.CIGAR())
}
func reverseComplement(b []byte) []byte {
	r := make([]byte, len(b))
	for i, c := range b {
		switch c {
		case 'A':
			c = 'T'
		case 'C':
			c = 'G'
		case 'G':
			c = 'C'
		case 'T':
			c = 'A'
		default:
			c = 'N'
		}
		r[len(b)-1-i] = c
	}
	return r
}
func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
func exact(a, b byte) bool { return a == b && a != 'N' }
func addOp(ops []Op, code byte, n int) []Op {
	if n <= 0 {
		return ops
	}
	if len(ops) > 0 && ops[len(ops)-1].Code == code {
		ops[len(ops)-1].Length += n
		return ops
	}
	return append(ops, Op{code, n})
}
