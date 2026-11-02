# Scalar, NEON and wavefront alignment study — 2026-10-23

A later [indexed-containment and sixteen-lane follow-up](OPTIMIZATION.md)
improves these results further while preserving every output path.

ARM64 NEON accelerates full-extension comparison by **2.09× on the original pair**
and **1.66× on the repeat-rich pair**, preserving byte-identical PAFs in both
cases. The unrolled portable scalar loop improves both by about 15%. WFA2 is
competitive with NEON in speed but its tested extension mode changes scores,
endpoints and paths. The Go WFA prototype and bounded hybrid do not beat NEON on these whole-genome runs.

Faqt and WFA rows are exploratory single-sample measurements per dataset/kernel.
The minimap2 follow-up rows are fresh medians of three single-worker runs per
dataset, with both input orders executed sequentially. This study does not
establish that WFA is intrinsically slower than SIMD DP. The earlier
[minimap2 report](RESULTS.md) records additional presets and repeat-placement
comparisons.

## Method

Apple M4 Pro, Go 1.27.2 darwin/arm64, `GOEXPERIMENT=simd`, `GOMAXPROCS=1`.
Source starts from `09dcd181d7328f169457583be1203f1be292e47a` and the existing uncommitted
end-extension experiment, with `EndExtensionDP` selected throughout. Every
kernel uses the same seed, chain, scoring, filtering and deduplication pipeline.
The anchor cap is 20M for every run; the repeat pair still exceeds faqt's 2M
default. Full 256-base end extension is retained; internal gaps are aligned.
The native adapter is benchmark-only and is not included in the production API.

Runs use fresh processes, sequentially with warm input caches. Tables show
both faqt comparison time and end-to-end process wall time. Compare the latter
across engines: it includes reading, indexing/preparation, alignment and PAF
output. Minimap2's two process times are summed. Compilation and later DNA
replay are excluded. Minimap2 has no separately instrumented faqt-style
comparison duration. Process wall/CPU, peak RSS, Go allocations, hashes and
placement comparisons are in the [machine-readable results](kernel-results-2026-10-23.json).
Reference runs additionally capture a periodic sample of calls. That capture
overhead is included in their times; it encodes at most 3,600 calls per dataset.
Other machine activity was not controlled; small timing differences, especially
the 2% repeat-rich difference between WFA2 and NEON, are inconclusive.

The original and repeat-rich FASTA hashes match the earlier benchmark. WFA2 is
version `v2.3`, main revision `6176f9476dcda0a2291dfff3248cedba2ba8d68e`,
built with `-O3 -march=native`. Its exact source archive SHA-256 is recorded in
the result JSON. The native runs share the same Go executable as other kernels.

## Single-worker minimap2 follow-up

Minimap2 `2.31-r1302` was rerun with `-t 1`, one compute worker per
process. Minimap2 can also use I/O pipeline threads; `-t 1` is its standard
single-worker setting. Both input orders ran sequentially, matching faqt's
bidirectional workflow. Seeds, scoring and repeat retention use the
[`mm-similar-both` profile](README.md#profiles), rather than stock `asm5`:

```text
-x asm5 -c --eqx -P -t 1 -k 15 -w 10 -A 2 -B 4 -O 4,24 -E 2,1
-g 2000 -r 2000,2000 -z 200,200 -m 40 -s 40 -n 1
-f 100000000 -U 100000000,100000000 --q-occ-frac 0 --no-long-join --rmq=no
```

Each dataset has three fresh-process samples, with two sequential processes
per sample. Output is byte-identical across repetitions within each direction.
Paths are normalized and deduplicated across orders under the same common
length/identity/score filters used in the assembly benchmark. The algorithms
still differ, including faqt retaining more alternative paths.

| Dataset | Median wall, both orders | Sample range | Median CPU | Peak RSS |
|---|---:|---:|---:|---:|
| Original | 2.764 s | 2.761–2.766 s | 2.718 s | 78.5 MiB |
| Repeats | 31.188 s | 31.183–31.266 s | 31.076 s | 566.7 MiB |

Using end-to-end wall time, NEON faqt remains about **4.53× slower on the
original pair** and **3.20× slower on repeats**. These combine the earlier
single faqt samples with the new three-run minimap2 medians; faqt was not rerun
for this follow-up. Output/placement differences preclude interpreting the
runtime ratio as a comparison of identical alignment results.

Reproduce with `benchmarks/assembly/run.py --profile mm-similar-both --runs 3`,
using the current debug directory. Exact commands, per-process resources and
input/output hashes are stored in the result JSON. Raw outputs were kept locally.

## Original pair

| Engine | Comparison only | End-to-end wall | Paths | Reference paired positions retained | Byte-identical PAF |
|---|---:|---:|---:|---:|---|
| Original scalar | 24.847 s | 28.247 s | 7,116 | 100.00000% | Yes |
| Unrolled scalar | 21.598 s | 21.744 s | 7,116 | 100.00000% | Yes |
| NEON DP | 11.887 s | 12.527 s | 7,116 | 100.00000% | Yes |
| Go WFA prototype | 136.697 s | 136.910 s | 7,070 | 99.84836% | No |
| Bounded Go WFA + NEON | 21.256 s | 21.465 s | 7,113 | 99.98715% | No |
| Native WFA2 reference | 13.049 s | 13.255 s | 6,973 | 99.30336% | No |
| minimap2 `-t 1`, both orders | — | **2.764 s** (median) | 4,928 | 90.24240% | No |

## Repeat-rich pair

| Engine | Comparison only | End-to-end wall | Paths | Reference paired positions retained | Byte-identical PAF |
|---|---:|---:|---:|---:|---|
| Original scalar | 165.173 s | 165.720 s | 42,414 | 100.00000% | Yes |
| Unrolled scalar | 140.798 s | 141.301 s | 42,414 | 100.00000% | Yes |
| NEON DP | 99.379 s | 99.848 s | 42,414 | 100.00000% | Yes |
| Bounded Go WFA + NEON | 112.421 s | 112.871 s | 42,428 | 99.99931% | No |
| Native WFA2 reference | 97.231 s | 97.702 s | 42,367 | 99.97722% | No |
| minimap2 `-t 1`, both orders | — | **31.188 s** (median) | 40,169 | 98.88054% | No |

Go WFA was not run on the entire repeat pair after its 5.5× slowdown on the
original pair. It was tested on captured repeat-rich calls. The bounded hybrid
and optimized native WFA2 reference were run on both complete pairs.

The reference for paired-position retention is the original faqt scalar output.
Paired positions identify both coordinates, contigs and strand. They include
alternative repeat and gap placements. A missing paired position is not
necessarily a newly uncovered genomic base or an incorrect alignment.
WFA2 loses no query/target coverage on the original pair and 27 query bases of
coverage on the repeat pair. The Go WFA and hybrid lose no one-dimensional
coverage on their completed runs. NEON and unrolled scalar preserve all paths
and coordinates exactly. All completed faqt/WFA PAFs pass independent DNA replay.

## Memory

NEON reduces Go allocation churn from 7.08 GB to 5.41 GB on the original pair,
and from 37.34 GB to 26.14 GB on the repeat pair. These are cumulative allocations,
not resident memory. Original-pair RSS falls from about 280 MB to 251 MB; repeat
RSS rises from about 1.05 GB to 1.29 GB. The kernel therefore does not establish
a peak-memory improvement on repeats.

The Go WFA prototype allocates about 281 GB on the original pair. The bounded
hybrid allocates about 54 GB and 181 GB respectively: failed cheap attempts can
add substantial allocation and GC work before the DP fallback. WFA2's Go
allocation figures exclude native allocations; use its process RSS when
comparing total memory.

## Similar synthetic sequences

WFA does win when the input fits its strengths. A direct-kernel benchmark with
about 2.7% substitutions, no indels and Z-drop disabled gives:

| Sequence length | Original scalar | NEON | Go WFA |
|---|---:|---:|---:|
| 32 | 18.0 µs | 8.6 µs | 15.4 µs |
| 128 | 235.7 µs | 88.6 µs | 33.0 µs |
| 256 | 671.9 µs | 212.7 µs | 64.3 µs |
| 1,000 | 2,874.1 µs | 934.1 µs | 514.5 µs |

These are 100 ms Go microbenchmarks, including traceback and allocations. The
1,000-base case exceeds the production end-extension limit and is a kernel
workload, not a whole-genome result. This supports investigating WFA for longer,
similar anchored gaps; it does not establish a win on the current short-flank
workload. Structured numbers are included in the result JSON.

## Why the outcomes differ

The NEON kernel computes four independent cells on each anti-diagonal, keeping
the five DP states and packed traceback. It reconstructs row maxima and then
applies the scalar row-order Z-drop and endpoint priorities. Earlier cells do
not depend on later rows, so this preserves the reference stopping behavior.
It computes band cells beyond a possible early drop, which limits its speedup
on divergent flanks. Large matrices, wide scoring ranges and restrictive cell
budgets fall back to scalar.

The Go WFA uses both affine gap costs and a match-reward transformation. Its
fixed-endpoint scores and optimal anchored prefixes are checked against scalar
DP with Z-drop disabled. With production Z-drop enabled, row-order stopping can
differ; equally scoring CIGARs can also differ. These changes can affect final
deduplication and filters even when most coordinate pairs agree.

The native reference uses WFA2's extension mode with maximum-score CIGAR trimming.
Captured original-pair calls show score differences in 99/3,600 calls and
endpoint differences in 104/3,600; the Go WFA has no score/endpoint differences
on that early sample but different tracebacks in 332 calls. Neither observation
guarantees equality on the full dataset. Of those original-pair native score
differences, 90 are extensions with lower prefix scores, and nine are internal
gaps whose results agree with scalar DP once Z-drop is disabled. The corresponding
repeat sample has three lower-scoring extensions and 17 gap differences that
agree after disabling Z-drop. Native extension endpoint differences also persist
without Z-drop. The native extension mode is not
assumed equivalent to faqt's best-prefix objective or row-order stopping.

WFA can be very effective for long, similar sequences. Here exact anchors have
already been maximally extended and compacted, while most end DP is restricted
to short unseeded flanks. My interpretation is that this removes much of WFA's
usual advantage and leaves relatively difficult decisions about whether any
extension is worthwhile. The Go prototype's allocation overhead is an
additional implementation limitation. This is a workload-specific observation,
not a general comparison of the algorithms. See the
[original WFA paper](https://pmc.ncbi.nlm.nih.gov/articles/PMC8355039/).

## Included changes and validation

- An unrolled scalar DP loop used by normal builds.
- Optional ARM64 NEON selected at build time by `GOEXPERIMENT=simd`, with a
  scalar fallback. Normal builds retain the existing Go version requirement;
  experimental SIMD was tested with Go 1.27.2.
- Randomized score/endpoint/traceback comparisons, fallback and cancellation
  checks, a fuzz target and kernel microbenchmarks.
- A separate [reproducible kernel harness](../kernels/README.md), including the
  Go WFA prototype and optional native WFA2 adapter. No production WFA/cgo
  dependency or runtime kernel-selection environment variable is added.
- Assembly benchmark filenames updated to the current `repeats.*` fixtures.

Normal and SIMD `go test ./...`, SIMD vet and race checks pass. The SIMD fuzz
target ran for 20 seconds. Linux/amd64 scalar, Linux/arm64 SIMD and Windows/arm64
SIMD test binaries cross-compile with cgo disabled; cross-compiled binaries were
not executed. The original missing-region, DNA replay and input-swap test also
passes with the production SIMD kernel. Whole-input scalar/NEON PAF hashes are
identical on both datasets, and every completed faqt/WFA candidate PAF was replayed.

The SIMD kernel changes no scoring or sensitivity options. Seqhiker still uses
its existing comparison implementation; connecting it to faqt is a separate
step. No commits were created by this study.

Large PAFs, captured sequence calls, binaries and raw logs were kept locally;
private DNA is not committed. Use the kernel harness for reproduction.
