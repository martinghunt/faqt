package randomcontigs

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/martinghunt/faqt/seqio"
)

type Options struct {
	Contigs       int
	Length        int
	NameByLetters bool
	Prefix        string
	Seed          *int64
	FirstNumber   int
}

func Generate(writer seqio.WriteCloser, opts Options) error {
	if writer == nil {
		return fmt.Errorf("writer must not be nil")
	}
	if opts.Contigs < 0 {
		return fmt.Errorf("contigs must be >= 0")
	}
	if opts.Length < 0 {
		return fmt.Errorf("length must be >= 0")
	}
	if opts.FirstNumber == 0 {
		opts.FirstNumber = 1
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	if opts.Seed != nil {
		rng = rand.New(rand.NewSource(*opts.Seed))
	}

	letters := []byte("ABCDEFGHIJKLMNOPQRSTUVWXYZ")
	lettersIndex := 0
	for i := 0; i < opts.Contigs; i++ {
		var name string
		if opts.NameByLetters {
			name = string(letters[lettersIndex])
			lettersIndex++
			if lettersIndex == len(letters) {
				lettersIndex = 0
			}
		} else {
			name = fmt.Sprint(i + opts.FirstNumber)
		}

		rec := &seqio.SeqRecord{
			Name: opts.Prefix + name,
			Seq:  randomSequence(rng, opts.Length),
		}
		if err := writer.Write(rec); err != nil {
			return err
		}
	}
	return nil
}

func GenerateToPath(path string, opts Options, writerOpts ...seqio.Option) (err error) {
	writer, err := seqio.CreateFASTAPath(path, writerOpts...)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := writer.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()
	return Generate(writer, opts)
}

func randomSequence(rng *rand.Rand, length int) []byte {
	seq := make([]byte, length)
	bases := []byte("ACGT")
	for i := range seq {
		seq[i] = bases[rng.Intn(len(bases))]
	}
	return seq
}
