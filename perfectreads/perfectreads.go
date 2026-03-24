package perfectreads

import (
	"fmt"
	"io"
	"math"
	"math/rand"

	"github.com/martinghunt/faqt/seq"
	"github.com/martinghunt/faqt/seqio"
)

type Options struct {
	MeanInsert int
	InsertStd  float64
	Coverage   float64
	ReadLength int
	NoN        bool
	Seed       int64
}

type Report struct {
	ReadsWritten int
	PairsWritten int
	SkippedShort []string
}

func GeneratePaired(reader seqio.Reader, forwardWriter, reverseWriter seqio.WriteCloser, opts Options) (Report, error) {
	if opts.MeanInsert <= 0 {
		return Report{}, fmt.Errorf("mean insert must be > 0")
	}
	if opts.InsertStd < 0 {
		return Report{}, fmt.Errorf("insert std deviation must be >= 0")
	}
	if opts.Coverage <= 0 {
		return Report{}, fmt.Errorf("coverage must be > 0")
	}
	if opts.ReadLength <= 0 {
		return Report{}, fmt.Errorf("read length must be > 0")
	}

	rng := rand.New(rand.NewSource(opts.Seed))
	report := Report{}
	pairCounter := 1

	for {
		ref, err := reader.Read()
		if err == io.EOF {
			return report, nil
		}
		if err != nil {
			return report, err
		}

		if len(ref.Seq) < opts.MeanInsert+int(4*opts.InsertStd) {
			report.SkippedShort = append(report.SkippedShort, ref.Name)
			continue
		}

		readPairs := int(0.5 * opts.Coverage * float64(len(ref.Seq)) / float64(opts.ReadLength))
		usedFragments := make(map[[2]int]int)

		x := 0
		attempts := 0
		maxAttempts := max(1000, readPairs*100)
		for x < readPairs {
			attempts++
			if attempts > maxAttempts {
				break
			}
			isize := int(rng.NormFloat64()*opts.InsertStd + float64(opts.MeanInsert))
			for isize > len(ref.Seq) || isize < opts.ReadLength {
				isize = int(rng.NormFloat64()*opts.InsertStd + float64(opts.MeanInsert))
			}
			middlePos := rng.Intn(int(math.Floor(float64(len(ref.Seq))-0.5*float64(isize)))-int(math.Ceil(0.5*float64(isize)))+1) + int(math.Ceil(0.5*float64(isize)))
			readStart1 := middlePos - int(math.Ceil(0.5*float64(isize)))
			readStart2 := readStart1 + isize - opts.ReadLength

			name := fmt.Sprintf("%s:%d:%d:%d", ref.Name, pairCounter, readStart1+1, readStart2+1)
			fragment := [2]int{middlePos, isize}
			if count, ok := usedFragments[fragment]; ok {
				count++
				usedFragments[fragment] = count
				name = fmt.Sprintf("%s.dup.%d", name, count)
			} else {
				usedFragments[fragment] = 1
			}

			read1Seq := append([]byte(nil), ref.Seq[readStart1:readStart1+opts.ReadLength]...)
			read2Seq := seq.ReverseComplement(ref.Seq[readStart2 : readStart2+opts.ReadLength])

			if opts.NoN && (containsN(read1Seq) || containsN(read2Seq)) {
				continue
			}

			rec1 := &seqio.SeqRecord{Name: name + "/1", Seq: read1Seq, Qual: qualityProfile(opts.ReadLength)}
			rec2 := &seqio.SeqRecord{Name: name + "/2", Seq: read2Seq, Qual: qualityProfile(opts.ReadLength)}

			if err := forwardWriter.Write(rec1); err != nil {
				return report, err
			}
			if err := reverseWriter.Write(rec2); err != nil {
				return report, err
			}

			pairCounter++
			x++
			report.PairsWritten++
		}
	}
}

func GenerateSingle(reader seqio.Reader, writer seqio.WriteCloser, opts Options) (Report, error) {
	if opts.Coverage <= 0 {
		return Report{}, fmt.Errorf("coverage must be > 0")
	}
	if opts.ReadLength <= 0 {
		return Report{}, fmt.Errorf("read length must be > 0")
	}

	rng := rand.New(rand.NewSource(opts.Seed))
	report := Report{}
	readCounter := 1

	for {
		ref, err := reader.Read()
		if err == io.EOF {
			return report, nil
		}
		if err != nil {
			return report, err
		}

		if len(ref.Seq) < opts.ReadLength {
			report.SkippedShort = append(report.SkippedShort, ref.Name)
			continue
		}

		readCount := int(opts.Coverage * float64(len(ref.Seq)) / float64(opts.ReadLength))
		usedStarts := make(map[int]int)

		x := 0
		attempts := 0
		maxAttempts := max(1000, readCount*100)
		for x < readCount {
			attempts++
			if attempts > maxAttempts {
				break
			}

			start := rng.Intn(len(ref.Seq) - opts.ReadLength + 1)
			name := fmt.Sprintf("%s:%d:%d", ref.Name, readCounter, start+1)
			if count, ok := usedStarts[start]; ok {
				count++
				usedStarts[start] = count
				name = fmt.Sprintf("%s.dup.%d", name, count)
			} else {
				usedStarts[start] = 1
			}

			readSeq := append([]byte(nil), ref.Seq[start:start+opts.ReadLength]...)
			if opts.NoN && containsN(readSeq) {
				continue
			}

			rec := &seqio.SeqRecord{Name: name, Seq: readSeq, Qual: qualityProfile(opts.ReadLength)}
			if err := writer.Write(rec); err != nil {
				return report, err
			}

			readCounter++
			x++
			report.ReadsWritten++
		}
	}
}

func qualityProfile(readLength int) []byte {
	q := make([]byte, readLength)
	for i := range q {
		q[i] = 'I'
	}
	tail := []byte("HGFEDCBA")
	n := len(tail)
	if readLength < n {
		tail = tail[n-readLength:]
		n = len(tail)
	}
	copy(q[readLength-n:], tail)
	return q
}

func containsN(seq []byte) bool {
	for _, b := range seq {
		if b == 'N' || b == 'n' {
			return true
		}
	}
	return false
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
