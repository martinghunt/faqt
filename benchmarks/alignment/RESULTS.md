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

Native [Ryzen/Linux full-mode measurements](../remote/RESULTS-2026-10-23.md)
show 3.50–3.70× speedup over scalar. Subsequent
[adaptive Linux checks](../remote/ADAPTIVE-2026-11-02.md) passed, including AVX2
and scalar fallback. Earlier [Windows checks](../remote/WINDOWS-2026-10-23.md)
cover full mode; the later adaptive changes have not been run natively there.
