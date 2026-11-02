# Adaptive end-extension selection — 2026-10-23

Selected implementation: opt-in `EndExtensionAdaptive`. Start with 64 bases per
flank, doubling to the available limit (normally 128 then 256). Continue if the
best endpoint reaches a truncated edge, or an edge path scores within 24 times
the match reward of the best prefix (48 points at default scoring). Stop on the
normal row-order Z-drop. Retries recompute from the original anchor, preserving
competing paths and affine-gap costs; they do not append independent chunks.
Boundary scores include only rows finalized before the scalar-order stop.
The portable, ARM64 NEON and amd64 AVX2 kernels share this rule.

Full DP remains the default. Internal gaps, seed density, scoring, repeat
retention rules and the 256-base maximum are unchanged. The adaptive rule is a
sensitivity/performance heuristic, not an equivalent-output optimisation.

## Selection and repeat validation

Screened 23 adaptive policies plus full control on the original pair, then seven
selected policies plus full control on repeats. Trials covered 32/48/64/96-base
starts, endpoint margins, boundary-score allowances, combined criteria and
allowances increasing with window size. A practical selection target was at least
99.95% reference paired-position retention on both fixtures, at most 100 total
uniquely covered bases lost per pair, and at least 1.5× speedup on both. The
48-point/64-base policy meets those observed targets. This is fixture-based
selection, not a universal sensitivity guarantee or held-out accuracy estimate.

The production implementation was then measured in three fresh processes per
mode/pair, alternating full/adaptive. Apple M4 Pro, Go 1.27.2, SIMD enabled,
GOMAXPROCS=1, K=15/W=10, anchor cap 20M. Comparison time excludes preparation
and PAF output. No other CPU-heavy checks ran during timings.

| Pair | Full median | Adaptive median | Speedup | Reference coordinate pairs retained | Unique A/B bases lost |
| --- | ---: | ---: | ---: | ---: | ---: |
| debug | 5.605 s | 3.147 s | 1.78× | 99.97109% | 0 / 0 |
| repeats | 24.971 s | 10.898 s | 2.29× | 99.98452% | 55 / 0 |

All distinct outputs passed independent DNA replay. Repetitions are byte-identical
within each mode/dataset. Full-mode PAFs exactly match the prior reference hashes.
The motivating missing-match regression, operation replay and input-swap symmetry
pass in both modes. Adaptive returns 7,116 / 42,333 paths on original/repeats;
full returns 7,116 / 42,414. Equal counts do not imply equal paths. Paired-position
retention measures reference agreement, not ground-truth correctness.

## Why this policy

Starting at 32 created extra retries and/or greater losses. Endpoint margins
helped, but observing boundary scores provided a better tradeoff. At default
scoring, a 32-point boundary allowance was faster (2.31 s original / 9.66 s
repeats in screening) but retained only 99.802% of original reference pairs.
64 points was nearly identical to reference (99.99987% / 99.99954%) but slower,
particularly on the original pair. Scaling the allowance with later windows and
combining it with endpoint margins did not improve the measured tradeoff enough
to justify added policy complexity. Only eight policies were run on repeats;
other table entries there are unmeasured, not failures.

## Full screening table

`m` is endpoint margin in bases; `f` is boundary-score deficit in default score
units. `scaled` increases that deficit proportionally at later stages. Timings
below are exploratory single samples and include prototype probe overhead.
Production medians above are the primary selected-mode measurements.

| Policy | Start | Margin | Deficit | Original seconds / retention | Repeats seconds / retention |
| --- | ---: | ---: | ---: | --- | --- |
| full | 256 | 0 | off | 5.584 / 100.00000% | 24.955 / 100.00000% |
| s32m0 | 32 | 0 | off | 1.272 / 97.52497% | not measured |
| s32m8 | 32 | 8 | off | 1.469 / 98.96384% | not measured |
| s32m16 | 32 | 16 | off | 1.605 / 99.37416% | 7.150 / 99.92804% |
| s64m0 | 64 | 0 | off | 1.773 / 99.15615% | 9.142 / 99.92208% |
| s64m8 | 64 | 8 | off | 1.812 / 99.45507% | not measured |
| s64m16 | 64 | 16 | off | 1.860 / 99.56708% | not measured |
| s64m32 | 64 | 32 | off | 2.116 / 99.64530% | not measured |
| s96m16 | 96 | 16 | off | 2.480 / 99.80148% | not measured |
| s64f16 | 64 | 0 | 16 | 1.985 / 99.72501% | 9.350 / 99.97616% |
| s64f32 | 64 | 0 | 32 | 2.306 / 99.80190% | 9.662 / 99.98336% |
| s64f64 | 64 | 0 | 64 | 4.366 / 99.99987% | 16.339 / 99.99954% |
| s32f16 | 32 | 0 | 16 | 1.688 / 99.66301% | not measured |
| s64f40 | 64 | 0 | 40 | 2.727 / 99.88614% | not measured |
| s64f48 | 64 | 0 | 48 | 3.211 / 99.97109% | 10.991 / 99.98452% |
| s64f56 | 64 | 0 | 56 | 3.834 / 99.99094% | not measured |
| s32f32 | 32 | 0 | 32 | 2.538 / 99.78325% | not measured |
| s32f48 | 32 | 0 | 48 | 3.592 / 99.97109% | not measured |
| s48f32 | 48 | 0 | 32 | 2.330 / 99.80769% | 9.277 / 99.98288% |
| s48f48 | 48 | 0 | 48 | 3.459 / 99.95665% | not measured |
| s64f32scaled | 64 | 0 | 32 | 2.601 / 99.81919% | not measured |
| s64f40scaled | 64 | 0 | 40 | 3.380 / 99.90323% | not measured |
| s64f32m16 | 64 | 16 | 32 | 2.309 / 99.80190% | not measured |
| s64f32m32 | 64 | 32 | 32 | 2.370 / 99.80989% | not measured |

## Validation and remaining limits

Portable and SIMD `go test ./...`, vet, race checks, and benchmark-harness tests
pass. New tests compare 900 randomized boundary/endpoint results between scalar
and platform kernels, including early Z-drop, small bands, empty inputs, wide
scores and fallbacks. Further tests cover concurrent calls, score scaling,
internal indels, repeat placements, cancellation, cell budgets, exact continuation
and the original full-input replay/swap regression in both modes.

A synthetic score-valley counterexample remains intentionally documented: the
64-base prefix can peak at 32 while the full 256-base search later recovers.
Adaptive mode can miss such recovery. Use EndExtensionDP when preserving the
full search is required. A safe stopping bound or resumable frontier implementation
is future work; no claim is made that these speedups preserve every full path.

Updated test binaries cross-compile for Linux/Windows amd64, Linux/Windows ARM64,
Darwin amd64 and portable Linux/386. Translated amd64 tests and an explicitly
AVX2-disabled scalar fallback pass on the development Mac. Translation backend
expectations varied during checking, so this is not counted as native AVX2
validation. A subsequent native Ryzen/Linux run on 2026-11-02 passed the
adaptive tests, full/adaptive original-genome regressions, and CPU-mask fallback
checks; see the [validation record](../remote/ADAPTIVE-2026-11-02.md).
Native Windows execution of the adaptive changes remains untested; the earlier
Windows full-mode results predate this change.

## Reproduction

The production assembly harness accepts `FAQT_BENCH_END_EXTENSION=adaptive`.
Its run.py profiles `faqt-full` and `faqt-adaptive` use matching raised anchor
budgets and one process each. They can run without minimap2:

```sh
GOEXPERIMENT=simd python3 benchmarks/assembly/run.py \
  --debug-dir "/path/to/Seqhiker_debug" --out /tmp/faqt-adaptive-check \
  --profile faqt-full --profile faqt-adaptive --runs 3
python3 benchmarks/assembly/analyze.py /tmp/faqt-adaptive-check
python3 benchmarks/assembly/verify.py /tmp/faqt-adaptive-check
```

Raw screening, validation and check metadata:
[adaptive-study-results-2026-10-23.json](adaptive-study-results-2026-10-23.json).
Isolated sources, drivers, PAFs and logs were kept locally during the study.
No private sequences or PAFs are committed.
