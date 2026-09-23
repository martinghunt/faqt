package minimizer

const invalidMinimizerHash = ^uint64(0)

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
		buffer[i].Hash = invalidMinimizerHash
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

		if valid == w+k-1 && min.Hash != invalidMinimizerHash {
			out = appendMatchingMinimizers(out, buffer[bufPos+1:], min)
			out = appendMatchingMinimizers(out, buffer[:bufPos], min)
		}

		if info.Hash <= min.Hash {
			if valid >= w+k && min.Hash != invalidMinimizerHash {
				out = append(out, min)
			}
			min = info
			minPos = bufPos
		} else if bufPos == minPos {
			if valid >= w+k-1 && min.Hash != invalidMinimizerHash {
				out = append(out, min)
			}
			min, minPos = windowMinimum(buffer, bufPos)
			if valid >= w+k-1 && min.Hash != invalidMinimizerHash {
				out = appendMatchingMinimizers(out, buffer[bufPos+1:], min)
				out = appendMatchingMinimizers(out, buffer[:bufPos+1], min)
			}
		}

		bufPos++
		if bufPos == w {
			bufPos = 0
		}
	}

	if min.Hash != invalidMinimizerHash {
		out = append(out, min)
	}
	return out
}

func invalidMinimizer() Minimizer {
	return Minimizer{Hash: invalidMinimizerHash}
}

// windowMinimum scans a circular window from the position after current
// through current. Using <= keeps the last equal minimum in that ring order,
// matching the tie-breaking used while the window advances.
func windowMinimum(buffer []Minimizer, current int) (Minimizer, int) {
	min := invalidMinimizer()
	minPos := 0
	for i := current + 1; i < len(buffer); i++ {
		if buffer[i].Hash <= min.Hash {
			min = buffer[i]
			minPos = i
		}
	}
	for i := 0; i <= current; i++ {
		if buffer[i].Hash <= min.Hash {
			min = buffer[i]
			minPos = i
		}
	}
	return min, minPos
}

func appendMatchingMinimizers(out, candidates []Minimizer, min Minimizer) []Minimizer {
	for _, candidate := range candidates {
		if sameMinimizer(min, candidate) {
			out = append(out, candidate)
		}
	}
	return out
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
