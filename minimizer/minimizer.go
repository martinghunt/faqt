package minimizer

import (
	"fmt"
	"io"

	"github.com/martinghunt/faqt/internal/closeutil"
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

func BuildFromPath(path string, opts Options) (idx *Index, err error) {
	reader, err := seqio.OpenPath(path)
	if err != nil {
		return nil, err
	}
	if closer, ok := reader.(io.Closer); ok {
		defer closeutil.CloseWithError(&err, closer)
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
