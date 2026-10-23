package align

import "context"

func alignEnd(ctx context.Context, query, target []byte, opts Options) (dpResult, error) {
	if opts.EndExtension == EndExtensionOneSNP {
		return extendOneSNP(ctx, query, target, opts)
	}
	if opts.EndExtension == EndExtensionAdaptive {
		return extendAdaptive(ctx, query, target, opts)
	}
	return alignDP(ctx, query, target, opts.BandWidth, true, opts)
}

// Exact anchors have already been maximally extended. A purely exact final
// scan would therefore add nothing. This cheap policy can cross one SNP, but
// does not attempt to recover indels or more divergent unseeded sequence.
func extendOneSNP(ctx context.Context, query, target []byte, opts Options) (dpResult, error) {
	if err := ctx.Err(); err != nil {
		return dpResult{}, err
	}
	score, bestScore, bestLen, mismatches := 0, 0, 0, 0
	for i := 0; i < min(len(query), len(target)); i++ {
		if i > 0 && i%4096 == 0 {
			if err := ctx.Err(); err != nil {
				return dpResult{}, err
			}
		}
		if query[i] == 'N' || target[i] == 'N' {
			break
		}
		if exact(query[i], target[i]) {
			score += opts.Match
		} else {
			if mismatches == 1 {
				break
			}
			mismatches++
			score -= opts.Mismatch
		}
		// Do not include a terminal SNP or a suffix that fails to improve the
		// anchored alignment's score. Score ties retain the shorter prefix.
		if score > bestScore {
			bestScore, bestLen = score, i+1
		}
	}
	result := dpResult{q: bestLen, t: bestLen, score: bestScore}
	for i := 0; i < bestLen; i++ {
		code := byte('X')
		if exact(query[i], target[i]) {
			code = '='
		}
		result.ops = addOp(result.ops, code, 1)
	}
	return result, nil
}

// Recompute each larger square from the anchor. Restarting from the best prefix
// would discard competing paths and alter affine-gap costs. Boundary scores are
// observed only through completed rows, preserving scalar/SIMD growth decisions.
func extendAdaptive(ctx context.Context, query, target []byte, o Options) (dpResult, error) {
	if err := ctx.Err(); err != nil {
		return dpResult{}, err
	}
	const start = 64
	maximum := max(len(query), len(target))
	// Avoid score-allowance overflow for extreme public scoring options.
	if maximum <= start || o.Match > int(^uint(0)>>1)/24 {
		return alignDP(ctx, query, target, o.BandWidth, true, o)
	}
	allowance := 24 * o.Match
	for limit := start; ; {
		boundary := dpBoundary{}
		r, err := alignDPPlatformBoundary(ctx, query[:min(len(query), limit)], target[:min(len(target), limit)], o.BandWidth, true, o, &boundary)
		if err != nil {
			return r, err
		}
		if boundary.dropped {
			return r, nil
		}
		growQ := len(query) > limit && (r.q == limit || (boundary.q != minusInfinity && boundary.q >= r.score-allowance))
		growT := len(target) > limit && (r.t == limit || (boundary.t != minusInfinity && boundary.t >= r.score-allowance))
		if !growQ && !growT {
			return r, nil
		}
		if limit >= maximum-limit {
			limit = maximum
		} else {
			limit *= 2
		}
	}
}
