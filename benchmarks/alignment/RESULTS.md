# Alignment benchmark results

Apple M4 Pro, macOS ARM64, Go 1.27.2 with NEON enabled, 2026-10-23. Median
total process seconds across three runs, one Go thread or minimap2 compute
worker; both input orders are included. Full/adaptive runs alternated. Minimap2
2.31-r1302 measurements were collected earlier on the same machine and reused.
The algorithms return different outputs; times do not compare identical work.

| Dataset | faqt full | faqt adaptive | minimap2 |
| --- | ---: | ---: | ---: |
| Original | 5.83 s | 3.35 s | 2.87 s |
| Repeats | 25.48 s | 11.40 s | 31.27 s |

Both faqt modes use K=15/W=10, a 20M anchor budget, default scoring, and a maximum
256-base outward extension. Internal gaps are fully aligned in both modes.
Minimap2 uses the closer seed/scoring/repeat settings documented in [README.md](README.md).

Adaptive mode starts with 64 bases and doubles its search while a boundary path
remains promising. It can miss recovery after a poor stretch, so full mode stays
the library default. Comparison-only time excludes loading/preparation/output:

| Dataset | Full | Adaptive | Speedup | Full coordinate pairs retained | Unique A/B bases lost |
| --- | ---: | ---: | ---: | ---: | ---: |
| Original | 5.605 s | 3.147 s | 1.78× | 99.97109% | 0 / 0 |
| Repeats | 24.971 s | 10.898 s | 2.29× | 99.98452% | 55 / 0 |

Full returns 7,116 / 42,414 paths; adaptive returns 7,116 / 42,333. Minimap2
returns 4,928 / 40,169 qualifying distinct paths. Equal counts do not mean equal
alignments. Coordinate-pair retention measures agreement with full faqt, not
ground-truth accuracy or a sensitivity guarantee on other inputs.

Every distinct faqt output passed independent DNA replay, and repetitions were
byte-identical within each mode/dataset. SIMD full-mode PAF hashes match the
scalar reference. Both modes recover the motivating missing match and preserve
exact input-swap symmetry. [results.json](results.json) retains the selected-mode
samples, medians, input hashes and reference output hashes; exploratory sweeps
and prototype kernels are omitted.

Native [Ryzen/Linux full-mode measurements](remote/RESULTS-2026-10-23.md)
show 3.50–3.70× speedup over scalar. Subsequent
[adaptive Linux checks](remote/ADAPTIVE-2026-11-02.md) passed, including AVX2
and scalar fallback. Earlier [Windows checks](remote/WINDOWS-2026-10-23.md)
cover full mode; the later adaptive changes have not been run natively there.

## Chaining and cleanup update (2026-11-04)

Chain extraction now claims anchors greedily from the strongest leaf, and
`RedundancyNear` adds end joining, a 95% containment threshold with internal
detours, and union trimming (see [align/README.md](../../align/README.md)).
Adaptive end extension, same fixtures and settings as below. Earlier tables
in this file, including the minimap2 comparisons, predate this change.

| Dataset | Policy | Paths before / after | Paths ≥10 kb paired, before / after | Sum of paired lengths, before / after |
| --- | --- | ---: | ---: | ---: |
| Original | exact | 7,116 / 7,121 | 37 / 33 | 4.57 M / 3.83 M |
| Original | near | 6,609 / 6,024 | 31 / 15 | 3.82 M / 3.24 M |
| Repeats | exact | 42,333 / 42,339 | 483 / 129 | 58.62 M / 40.85 M |
| Repeats | near | 39,933 / 39,501 | 137 / 83 | 38.60 M / 36.49 M |

Against the previous exact policy, the new exact policy retains 99.9279% /
99.9990% of distinct paired coordinates, and the new near policy retains
99.5106% / 99.9355% of the new exact policy's coordinates. Covered bases on both
genomes are unchanged in every comparison. All output paths replay against DNA.
Comparison time is unchanged within run-to-run variation (about 3.1 s / 10.8 s,
one thread). Details: [chaining-study](chaining-study/README.md) and
[release-summary.json](chaining-study/release-summary.json).

## Near-duplicate cleanup

The opt-in `RedundancyNear` policy was checked on the same original and repeat
fixtures. These are output comparisons, not repeated speed measurements.

| Dataset | Adaptive exact policy | Adaptive near policy | Paths with ≥10 kb paired sequence, before / after | Original paired coordinates retained |
| --- | ---: | ---: | ---: | ---: |
| Original | 7,116 | 6,609 | 37 / 31 | 99.9141% |
| Repeats | 42,333 | 39,933 | 483 / 137 | 99.9541% |

Covered bases on both genomes are unchanged on these fixtures. Normalisation
moves some paired coordinates to equivalent short-gap placements; suppression
also discards small alternative pieces. Consequently these percentages measure
agreement with the original paths, not biological sensitivity. All output paths
passed independent DNA replay. The motivating missing-region regression and
exact input-swap symmetry pass, as do synthetic Cartesian and tandem-repeat-copy
checks. The library default remains `RedundancyExact`.

In the repeat fixture, the sum of paired lengths across all hits falls from
58.62 million to 38.60 million, removing much of the long-path overplotting.
Distinct repeat placements still produce many hits. Compact measurements are in
[redundancy-results.json](redundancy-results.json); earlier reference snapshots
continue to describe the unchanged exact policy.
