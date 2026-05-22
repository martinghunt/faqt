package minimizer

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
