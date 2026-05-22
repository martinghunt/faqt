package minimizer

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
