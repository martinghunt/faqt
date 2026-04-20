package minimizer

import (
	"fmt"
	"io"

	"github.com/martinghunt/faqt/seqio"
)

type Options struct {
	K         int
	W         int
	MidOcc    int
	MaxMaxOcc int
	OccDist   int
	QOccFrac  float64
}

type RefPos struct {
	RefID  int
	Pos    int
	Strand uint8
}

type Minimizer struct {
	Hash   uint64
	Pos    int
	Strand uint8
}

type Index struct {
	K               int
	W               int
	MidOcc          int
	MaxMaxOcc       int
	OccDist         int
	QOccFrac        float64
	Refs            []seqio.SeqRecord
	Table           map[uint64][]RefPos
	TotalMinimizers int
}

func Build(reader seqio.Reader, opts Options) (*Index, error) {
	if err := opts.validate(); err != nil {
		return nil, err
	}

	idx := &Index{
		K:         opts.K,
		W:         opts.W,
		MidOcc:    opts.MidOcc,
		MaxMaxOcc: opts.MaxMaxOcc,
		OccDist:   opts.OccDist,
		QOccFrac:  opts.QOccFrac,
		Table:     make(map[uint64][]RefPos),
	}

	for {
		rec, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		refID := len(idx.Refs)
		stored := seqio.SeqRecord{
			Name:        rec.Name,
			Description: rec.Description,
			Seq:         append([]byte(nil), rec.Seq...),
		}
		if rec.Qual != nil {
			stored.Qual = append([]byte(nil), rec.Qual...)
		}
		idx.Refs = append(idx.Refs, stored)

		mins := Sketch(rec.Seq, opts.K, opts.W)
		idx.TotalMinimizers += len(mins)
		for _, m := range mins {
			idx.Table[m.Hash] = append(idx.Table[m.Hash], RefPos{
				RefID:  refID,
				Pos:    m.Pos,
				Strand: m.Strand,
			})
		}
	}
	return idx, nil
}

func BuildFromPath(path string, opts Options) (*Index, error) {
	reader, err := seqio.OpenPath(path)
	if err != nil {
		return nil, err
	}
	if closer, ok := reader.(io.Closer); ok {
		defer closer.Close()
	}
	return Build(reader, opts)
}

func (i *Index) Lookup(hash uint64) []RefPos {
	hits := i.Table[hash]
	if len(hits) == 0 {
		return nil
	}
	return hits
}

func (i *Index) LookupMinimizer(m Minimizer) []RefPos {
	return i.Lookup(m.Hash)
}

type Anchor struct {
	Hash        uint64
	QueryPos    int
	QueryStrand uint8
	RefID       int
	RefPos      int
	RefStrand   uint8
	Occurrence  int
}

func (i *Index) Query(seq []byte) []Anchor {
	mins := Sketch(seq, i.K, i.W)
	mins = i.filterQueryDuplicates(mins)
	selected := i.selectQueryMinimizers(mins, len(seq))

	anchors := make([]Anchor, 0, len(selected))
	for _, m := range selected {
		hits := i.Lookup(m.Hash)
		for _, hit := range hits {
			anchors = append(anchors, Anchor{
				Hash:        m.Hash,
				QueryPos:    m.Pos,
				QueryStrand: m.Strand,
				RefID:       hit.RefID,
				RefPos:      hit.Pos,
				RefStrand:   hit.Strand,
				Occurrence:  len(hits),
			})
		}
	}
	return anchors
}

func Sketch(seq []byte, k, w int) []Minimizer {
	if err := (Options{K: k, W: w}).validate(); err != nil {
		panic(err)
	}
	if len(seq) == 0 {
		return nil
	}

	mask := uint64(1<<(2*k)) - 1
	shift := uint(2 * (k - 1))
	forward, reverse := uint64(0), uint64(0)
	valid := 0

	buffer := make([]Minimizer, w)
	for i := range buffer {
		buffer[i].Hash = ^uint64(0)
	}
	var out []Minimizer
	bufPos := 0
	minPos := 0
	min := invalidMinimizer()

	for i, b := range seq {
		info := invalidMinimizer()
		code, ok := nt4(b)
		if ok {
			forward = ((forward << 2) | uint64(code)) & mask
			reverse = (reverse >> 2) | (uint64(3^code) << shift)
			valid++
			if valid >= k && forward != reverse {
				strand := uint8(0)
				canonical := forward
				if reverse < forward {
					canonical = reverse
					strand = 1
				}
				info = Minimizer{
					Hash:   hash64(canonical) & mask,
					Pos:    i - k + 1,
					Strand: strand,
				}
			}
		} else {
			forward, reverse = 0, 0
			valid = 0
			min = invalidMinimizer()
		}

		buffer[bufPos] = info

		if valid == w+k-1 && min.Hash != ^uint64(0) {
			for j := bufPos + 1; j < w; j++ {
				if sameMinimizer(min, buffer[j]) {
					out = append(out, buffer[j])
				}
			}
			for j := 0; j < bufPos; j++ {
				if sameMinimizer(min, buffer[j]) {
					out = append(out, buffer[j])
				}
			}
		}

		if info.Hash <= min.Hash {
			if valid >= w+k && min.Hash != ^uint64(0) {
				out = append(out, min)
			}
			min = info
			minPos = bufPos
		} else if bufPos == minPos {
			if valid >= w+k-1 && min.Hash != ^uint64(0) {
				out = append(out, min)
			}
			min = invalidMinimizer()
			for j := bufPos + 1; j < w; j++ {
				if buffer[j].Hash <= min.Hash {
					min = buffer[j]
					minPos = j
				}
			}
			for j := 0; j <= bufPos; j++ {
				if buffer[j].Hash <= min.Hash {
					min = buffer[j]
					minPos = j
				}
			}
			if valid >= w+k-1 && min.Hash != ^uint64(0) {
				for j := bufPos + 1; j < w; j++ {
					if sameMinimizer(min, buffer[j]) {
						out = append(out, buffer[j])
					}
				}
				for j := 0; j <= bufPos; j++ {
					if sameMinimizer(min, buffer[j]) {
						out = append(out, buffer[j])
					}
				}
			}
		}

		bufPos++
		if bufPos == w {
			bufPos = 0
		}
	}

	if min.Hash != ^uint64(0) {
		out = append(out, min)
	}
	return out
}

func (o Options) validate() error {
	if o.K <= 0 {
		return fmt.Errorf("k must be > 0")
	}
	if o.K > 31 {
		return fmt.Errorf("k must be <= 31")
	}
	if o.W <= 0 {
		return fmt.Errorf("w must be > 0")
	}
	if o.W >= 256 {
		return fmt.Errorf("w must be < 256")
	}
	if o.MidOcc < 0 {
		return fmt.Errorf("mid occurrence must be >= 0")
	}
	if o.MaxMaxOcc < 0 {
		return fmt.Errorf("max-max occurrence must be >= 0")
	}
	if o.MaxMaxOcc > 0 && o.MidOcc > 0 && o.MaxMaxOcc < o.MidOcc {
		return fmt.Errorf("max-max occurrence must be >= mid occurrence")
	}
	if o.OccDist < 0 {
		return fmt.Errorf("occurrence distance must be >= 0")
	}
	if o.QOccFrac < 0 || o.QOccFrac > 1 {
		return fmt.Errorf("query occurrence fraction must be in [0,1]")
	}
	return nil
}

type querySeed struct {
	Minimizer
	occ int
}

func (i *Index) filterQueryDuplicates(mins []Minimizer) []Minimizer {
	if i.MidOcc <= 0 || i.QOccFrac <= 0 || len(mins) <= i.MidOcc {
		return mins
	}

	counts := make(map[uint64]int, len(mins))
	for _, m := range mins {
		counts[m.Hash]++
	}

	limit := float64(len(mins)) * i.QOccFrac
	out := mins[:0]
	for _, m := range mins {
		cnt := counts[m.Hash]
		if cnt > i.MidOcc && float64(cnt) > limit {
			continue
		}
		out = append(out, m)
	}
	return out
}

func (i *Index) selectQueryMinimizers(mins []Minimizer, qlen int) []Minimizer {
	if len(mins) == 0 {
		return nil
	}

	seeds := make([]querySeed, 0, len(mins))
	for _, m := range mins {
		occ := len(i.Lookup(m.Hash))
		if occ == 0 {
			continue
		}
		seeds = append(seeds, querySeed{Minimizer: m, occ: occ})
	}
	if len(seeds) == 0 {
		return nil
	}
	if i.MidOcc <= 0 {
		out := make([]Minimizer, 0, len(seeds))
		for _, s := range seeds {
			out = append(out, s.Minimizer)
		}
		return out
	}

	keep := make([]bool, len(seeds))
	for idx, s := range seeds {
		if s.occ <= i.MidOcc {
			keep[idx] = true
		}
	}

	for start, lastLow := 0, -1; start <= len(seeds); start++ {
		if start == len(seeds) || seeds[start].occ <= i.MidOcc {
			if start-lastLow > 1 {
				streakStart := lastLow + 1
				streakEnd := start
				ps := 0
				if lastLow >= 0 {
					ps = seeds[lastLow].Pos
				}
				pe := qlen
				if start < len(seeds) {
					pe = seeds[start].Pos
				}
				i.rescueHighOccurrenceSeeds(seeds, keep, streakStart, streakEnd, pe-ps)
			}
			lastLow = start
		}
	}

	out := make([]Minimizer, 0, len(seeds))
	for idx, s := range seeds {
		if keep[idx] {
			out = append(out, s.Minimizer)
		}
	}
	return out
}

func (i *Index) rescueHighOccurrenceSeeds(seeds []querySeed, keep []bool, start, end, span int) {
	if i.OccDist <= 0 || i.MaxMaxOcc <= i.MidOcc || span <= 0 {
		return
	}

	maxHigh := int(float64(span)/float64(i.OccDist) + 0.5)
	if maxHigh <= 0 {
		return
	}
	if maxHigh > end-start {
		maxHigh = end - start
	}

	chosen := make([]int, 0, maxHigh)
	for idx := start; idx < end; idx++ {
		if seeds[idx].occ > i.MaxMaxOcc {
			continue
		}
		if len(chosen) < maxHigh {
			chosen = append(chosen, idx)
			continue
		}
		worst := 0
		for j := 1; j < len(chosen); j++ {
			if seeds[chosen[j]].occ > seeds[chosen[worst]].occ {
				worst = j
			}
		}
		if seeds[idx].occ < seeds[chosen[worst]].occ {
			chosen[worst] = idx
		}
	}

	for _, idx := range chosen {
		keep[idx] = true
	}
}

func invalidMinimizer() Minimizer {
	return Minimizer{Hash: ^uint64(0)}
}

func sameMinimizer(a, b Minimizer) bool {
	return a.Hash == b.Hash && a.Pos != b.Pos
}

func nt4(b byte) (uint8, bool) {
	switch b {
	case 'A', 'a':
		return 0, true
	case 'C', 'c':
		return 1, true
	case 'G', 'g':
		return 2, true
	case 'T', 't':
		return 3, true
	default:
		return 0, false
	}
}

func hash64(key uint64) uint64 {
	key = ^key + (key << 21)
	key ^= key >> 24
	key = key + (key << 3) + (key << 8)
	key ^= key >> 14
	key = key + (key << 2) + (key << 4)
	key ^= key >> 28
	key += key << 31
	return key
}
