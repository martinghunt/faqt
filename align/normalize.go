package align

import (
	"context"
	"slices"
)

// normalizeMatch left-aligns short indels across verified exact matches only
// when the complete possible shift is shorter than K. Longer alternatives can
// describe genuine tandem-repeat placements and are left unchanged. Both
// intervals and every aligned base remain valid. Removing an empty exact run
// can merge adjacent gaps; recompute the score if that happens.
func normalizeMatch(m Match, a, b *Genome, o Options) Match {
	query := a.contigs[m.AContig].bases
	target := b.contigs[m.BContig].bases
	q, t := m.A.Start, m.B.Start
	if m.Reverse {
		target = b.contigs[m.BContig].reverse
		t = len(target) - m.B.End
	}
	ops := m.Ops
	changed := false
	for i := range ops {
		op := ops[i]
		if op.Length < a.sketch.K && (op.Code == 'I' || op.Code == 'D') && i > 0 && i+1 < len(ops) && ops[i-1].Code == '=' && ops[i+1].Code == '=' {
			shift := 0
			for shift < ops[i-1].Length && shift < a.sketch.K {
				if op.Code == 'I' {
					if !exact(query[q-1-shift], query[q+op.Length-1-shift]) {
						break
					}
				} else if !exact(target[t-1-shift], target[t+op.Length-1-shift]) {
					break
				}
				shift++
			}
			if shift != 0 && shift < a.sketch.K {
				if !changed {
					ops = slices.Clone(ops)
					changed = true
				}
				ops[i-1].Length -= shift
				ops[i+1].Length += shift
				q -= shift
				t -= shift
			}
		}
		if op.Code != 'D' {
			q += op.Length
		}
		if op.Code != 'I' {
			t += op.Length
		}
	}
	if !changed {
		return m
	}
	m.Ops = nil
	for _, op := range ops {
		m.Ops = addOp(m.Ops, op.Code, op.Length)
	}
	m.Matches, m.Columns, m.Score = 0, 0, 0
	for _, op := range m.Ops {
		m.Columns += op.Length
		switch op.Code {
		case '=':
			m.Matches += op.Length
			m.Score += op.Length * o.Match
		case 'X':
			m.Score -= op.Length * o.Mismatch
		default:
			m.Score -= gapCost(op.Length, o)
		}
	}
	return m
}

func normalizeMatches(ctx context.Context, a, b *Genome, matches []Match, o Options) ([]Match, error) {
	self := a.digest == b.digest
	for i, m := range matches {
		if i%64 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		// Equal-content genomes have no distinguished axis. Normalize one
		// representative per swap orbit; restore the symmetric set afterwards.
		if self {
			if swapped := m.Swap(); compareMatch(swapped, m) < 0 {
				m = swapped
			}
		}
		matches[i] = normalizeMatch(m, a, b, o)
	}
	return matches, nil
}
