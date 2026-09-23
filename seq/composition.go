package seq

// Composition counts the bases of a sequence. Case is ignored and U counts as
// T, so soft-masked and RNA sequence count the same as unmasked DNA.
type Composition struct {
	A     int
	C     int
	G     int
	T     int
	N     int
	NRuns int // runs of one or more consecutive Ns
}

// CountComposition counts the bases of one sequence in a single pass.
func CountComposition(s []byte) Composition {
	var c Composition
	inN := false
	for _, b := range s {
		switch b {
		case 'A', 'a':
			c.A++
		case 'C', 'c':
			c.C++
		case 'G', 'g':
			c.G++
		case 'T', 't', 'U', 'u':
			c.T++
		case 'N', 'n':
			c.N++
			if !inN {
				c.NRuns++
				inN = true
			}
			continue
		}
		inN = false
	}
	return c
}

// Add accumulates other into c. Runs of Ns are counted per sequence, so a
// sequence ending in Ns and the next one starting with Ns stay two runs.
func (c *Composition) Add(other Composition) {
	c.A += other.A
	c.C += other.C
	c.G += other.G
	c.T += other.T
	c.N += other.N
	c.NRuns += other.NRuns
}

// ACGT returns the number of unambiguous bases counted, which is the
// denominator GCPercent divides by.
func (c Composition) ACGT() int {
	return c.A + c.C + c.G + c.T
}

// GCPercent returns G+C as a percentage of A+C+G+T. Ns, gaps and ambiguity
// codes are left out of the denominator, the same convention as seqkit
// fx2tab --gc, so N content does not drag the reported GC down. Sequence with
// no unambiguous bases gives zero.
func (c Composition) GCPercent() float64 {
	total := c.ACGT()
	if total == 0 {
		return 0
	}
	return 100 * float64(c.G+c.C) / float64(total)
}
