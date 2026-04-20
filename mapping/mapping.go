package mapping

import (
	"fmt"
	"io"

	"github.com/martinghunt/faqt/align"
	"github.com/martinghunt/faqt/mapper"
	"github.com/martinghunt/faqt/minimizer"
	"github.com/martinghunt/faqt/seqio"
)

type Result struct {
	QueryName   string
	QueryLength int
	Hits        []Hit
}

type Hit struct {
	RefName        string
	RelativeStrand uint8
	Chain          mapper.Chain
	Candidate      mapper.Candidate
	Alignment      align.Result
}

type Mapper struct {
	Index         *minimizer.Index
	Pipeline      mapper.Pipeline
	CandidateOpts mapper.CandidateOptions
	Aligner       align.Aligner
	AlignOpts     align.Options
}

type ReaderResult struct {
	Results []Result
}

func New(index *minimizer.Index) *Mapper {
	defaultAligner := align.DefaultAligner()
	return &Mapper{
		Index:         index,
		Pipeline:      mapper.DefaultPipeline(),
		CandidateOpts: mapper.CandidateOptions{MaxCandidates: 5},
		Aligner:       defaultAligner,
		AlignOpts:     defaultAligner.Options,
	}
}

func BuildFromReader(reader seqio.Reader, opts minimizer.Options) (*Mapper, error) {
	index, err := minimizer.Build(reader, opts)
	if err != nil {
		return nil, err
	}
	return New(index), nil
}

func BuildFromPath(path string, opts minimizer.Options) (*Mapper, error) {
	index, err := minimizer.BuildFromPath(path, opts)
	if err != nil {
		return nil, err
	}
	return New(index), nil
}

func (m *Mapper) Map(queryName string, query []byte) (Result, error) {
	if m == nil {
		return Result{}, fmt.Errorf("mapper is nil")
	}
	if m.Index == nil {
		return Result{}, fmt.Errorf("index is required")
	}

	candidates, err := mapper.Map(m.Index, query, m.Pipeline, m.CandidateOpts)
	if err != nil {
		return Result{}, err
	}

	var hits []Hit
	if m.Aligner == nil {
		hits = make([]Hit, len(candidates))
		for i, candidate := range candidates {
			hits[i] = Hit{
				RefName:        candidate.RefName,
				RelativeStrand: candidate.RelativeStrand,
				Chain:          candidate.Chain,
				Candidate:      candidate,
			}
		}
	} else {
		alignments, err := align.AlignCandidates(m.Aligner, candidates, m.AlignOpts)
		if err != nil {
			return Result{}, err
		}
		hits = make([]Hit, len(alignments))
		for i, aln := range alignments {
			hits[i] = Hit{
				RefName:        aln.Candidate.RefName,
				RelativeStrand: aln.Candidate.RelativeStrand,
				Chain:          aln.Candidate.Chain,
				Candidate:      aln.Candidate,
				Alignment:      aln,
			}
		}
	}

	return Result{
		QueryName:   queryName,
		QueryLength: len(query),
		Hits:        hits,
	}, nil
}

func (m *Mapper) MapRecord(rec *seqio.SeqRecord) (Result, error) {
	if rec == nil {
		return Result{}, fmt.Errorf("query record is nil")
	}
	return m.Map(rec.Name, rec.Seq)
}

func (m *Mapper) MapAll(reader seqio.Reader) ([]Result, error) {
	if reader == nil {
		return nil, fmt.Errorf("reader is nil")
	}

	results := make([]Result, 0, 16)
	for {
		rec, err := reader.Read()
		if err == io.EOF {
			return results, nil
		}
		if err != nil {
			return nil, err
		}
		result, err := m.MapRecord(rec)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
}
