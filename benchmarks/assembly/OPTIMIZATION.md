# Indexed containment and score-difference SIMD — 2026-10-23

The combined changes preserve every previous path and full end extension.
Comparison time improves by about **2.1× on the original pair** and **3.7× on
the repeat-rich pair** relative to the previous four-lane SIMD implementation.
Against a fresh single-worker minimap2 comparison, faqt is about twice as slow
on the original pair and about 1.27× faster on this repeat-rich pair. These are
observations on these fixtures and configurations, not a general speed ranking
or a claim of identical cross-engine accuracy.

## Complete-pair timings

| Configuration | Runs per pair | Original: comparison | Original: wall | Repeats: comparison | Repeats: wall |
|---|---:|---:|---:|---:|---:|
| Before: four-lane DP and scans | 1 | 11.299 s | 12.068 s | 89.724 s | 90.360 s |
| Index only | 1 | 10.751 s | 11.859 s | 48.529 s | 49.048 s |
| Score-difference DP only | 1 | 5.765 s | 6.897 s | 64.405 s | 64.916 s |
| Both optimizations | 3 | 5.427 s | 5.637 s | 24.064 s | 24.572 s |
| minimap2 `-t 1`, both orders | 3 | — | 2.872 s | — | 31.272 s |

Every faqt run returns exactly 7,116 paths on the original pair and 42,414 on
repeats. PAFs are byte-identical to the original scalar reference across all
variants and repetitions. Minimap2 produces 4,928 and 40,169 distinct paths
after the common output filters; its algorithms and path selection differ.

The combined faqt and minimap2 rows are medians of three fresh-process samples.
The baseline and component-only rows are single exploratory samples. Compare
end-to-end wall columns across engines: they include reading, preparation/indexing,
alignment and output. The comparison columns instrument only faqt's Compare.
Compilation, tests and later analysis are excluded. Other CPU-intensive checks
were not run concurrently with timing. The original pair's first combined run
had additional wall time; its comparison/CPU time agrees with subsequent runs.
Raw sample ranges, CPU, peak RSS, allocations, exact commands and hashes are in
the [machine-readable results](optimization-results-2026-10-23.json).

## Method

Apple M4 Pro, Go 1.27.2 darwin/arm64, `GOEXPERIMENT=simd`, `GOMAXPROCS=1`.
All faqt cases retain `EndExtensionDP`, the same scoring, 256-base end extension,
adaptive gap alignment, filters, input-swap symmetry and repeat policies. The
anchor cap is 20M in every case. The repeat pair still exceeds the unchanged
2M default. Inputs match the hashes used in the previous kernel study.

Minimap2 2.31-r1302 uses `-t 1` and the repeat-retaining `mm-similar-both`
settings in [the assembly benchmark](README.md#profiles). Each sample sums two
sequential processes with reversed input orders. The one-compute-worker setting
may still create I/O pipeline threads. Outputs are byte-identical across all
three repetitions within each direction and match the prior minimap2 PAF hashes.
The new minimap2 run follows the faqt runs on the same machine.

## Containment index

Both the standalone-exact-run check and final subpath deduplication previously
scanned earlier retained matches. An exact subpath must begin on its containing
path. The index organizes possible start points by contig pair, strand and:

- Paired-base diagonal, with an interval on A.
- Insertion: fixed oriented B coordinate, with an interval on A.
- Deletion: fixed A coordinate, with an interval on oriented B.

Augmented interval treaps select candidates at that point. Consecutive paired
operations can share an index interval even across substitutions. Every selected
candidate still passes the original full `pathContains` check. The index does
not infer redundancy from overlapping intervals, impose a placement cap, consume
shared anchors or change the CIGAR equality contract. Endpoint-inclusive index
intervals preserve starts exactly on operation boundaries. Reverse B coordinates
use the same orientation as the existing containment predicate.

The new index is used by normal portable builds too. Its tests compare queries
and complete retained sets against the original scans, including starts inside
gaps, reversed paths, duplicate placements and differing CIGARs.

## Sixteen-lane DP and bookkeeping

The optional ARM64 kernel uses the
[Suzuki-Kasahara difference recurrence](https://academic.oup.com/bioinformatics/article/34/18/3094/4994778),
with sixteen 8-bit lanes for local score differences. Absolute endpoint scores
remain 32-bit. Conservative parameter checks select the kernel only when its
intermediates fit; short/narrow or oversized inputs, unsupported scores and
restrictive cell budgets use the existing wide/scalar fallback.

Traceback stores one byte per cell: the best state and four gap-continuation
flags. The predecessor is recovered from its source cell. This preserves the
original five-state priorities, continuation-on-tie decisions and final RLE
operations. Vector updates also maintain row maxima and their first attaining
coordinates, replacing the per-cell bookkeeping callback.

Rows are finalized as soon as their last band cell is computed. Z-drop and
endpoint selection consume those completed rows in the original scalar order;
partially visited later rows cannot affect the decision. The kernel can stop
before completing the remaining band while preserving the previous row-order
semantics. Scratch arrays are pooled; oversized buffers are not retained.
Returned CIGAR operations never borrow pooled storage. No cgo, external executable
or handwritten assembly is added to the library.

## Allocation and memory

Median cumulative Go allocations fall from about 5.41 GB to 0.38 GB on the
original pair and from 26.14 GB to 3.52 GB on repeats. The index adds some storage
of its own, but pooled DP scratch removes most of the previous allocation churn.
Peak RSS remains substantial on repeats: about 1.24 GB across the three combined
runs, versus about 1.38 GB in the fresh baseline. These are process peaks, not
allocation totals; do not interpret lower allocation churn as a matching drop
in resident memory. Raw seed-anchor storage and its default cap are unchanged.

## Verification and reproduction

- Normal and SIMD `go test ./...`; targeted vet and alignment race checks.
- 7,200 captured calls retain exact score, endpoint, operations and band-edge flag.
- About 720,000 SIMD/scalar fuzz executions and 80,000 whole-comparison fuzz
  executions, with replay and input-swap checks.
- The original missing-region regression passes, replays every Match including
  its score, and verifies exact equality after swapping inputs.
- Every timed faqt PAF matches the previously independently replayed scalar
  PAF. Input hashes are unchanged, so this also preserves that DNA replay result.
- Linux/amd64 scalar, Linux/arm64 SIMD and Windows/arm64 SIMD test binaries
  cross-compile with cgo disabled. Non-native binaries were not executed.
- The packaged benchmark builds and oracle tests pass for both indexed and
  scan-based copies; `wide`/scan and `simd`/indexed command runs reproduce the
  original-pair reference PAF.

The [kernel harness](../kernels/README.md#index-and-dp-ablation) now supports
`--scan-containment` and `--kernel wide`. Build indexed and scan-based source
copies, then run `wide` and `simd` on each to reproduce the four combinations.
The scan templates are benchmark-only and never enter the normal library.
Normal builds remain portable; enable SIMD with `GOEXPERIMENT=simd` on a
supporting Go toolchain on ARM64 or amd64. Unsupported configurations retain
scalar behavior. Native amd64 validation is recorded in the
[Linux results](../remote/RESULTS-2026-10-23.md).

Private FASTAs and PAFs are not committed. Raw runs, overlays and binaries were
kept locally during the study; the linked results JSON preserves the measurements.
Seqhiker integration is separate from this library change.
