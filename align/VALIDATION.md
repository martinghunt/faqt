# Assembly rewrite validation

Validation performed on 2026-10-23 on an Apple M4 Pro. These are observations of
this branch, not a general accuracy or performance guarantee.

## Automated checks

- Full `go test ./...` and `go vet ./...`.
- Alignment tests under the race detector, including concurrent reuse of prepared
  genomes.
- More than 20,000 fuzz executions covering DNA edits, reverse complements,
  duplicated sequences, operation replay, and input swapping.
- `CGO_ENABLED=0` test-binary cross-compilation for Linux, macOS and Windows on
  amd64/arm64, plus Linux/386. Non-native binaries were compiled, not executed.
- Independent small-matrix oracle for two-piece affine DP scores and traceback.

## Original Seqhiker debug pair

The private input sequences are not committed to the repository:

- `31585_6#49.fasta`: 18 contigs, 2,084,040 bases.
- `barcode10_ICESp23_spliced.fasta`: one contig, 2,095,267 bases.

With default options, an observed comparison took approximately 26 seconds,
excluding genome preparation, and returned 7,116 local alignment paths. All paths
were replayed against the original sequences: coordinate consumption, exact
matches, substitutions, indels, column counts and alignment scores agreed.
Input swapping produced exactly identical results after swapping axes and
operations. DP performance optimizations preserved every result and operation. The final
adaptive-band correctness fix recovered one additional path without removing
any earlier result.

The missing region on `.31585_6_49.4` is recovered, including the tandem-repeat
boundary. One recovered reverse-strand match spans zero-based query interval
`[66381,69732)` at approximately 97.92% identity, with a 3,078-base exact run.
Several alternative repeat placements are retained.

The local minimap2 executable was version `2.31-r1302`. A baseline run used:

```sh
minimap2 -x asm5 -c --eqx -P barcode10_ICESp23_spliced.fasta '31585_6#49.fasta'
```

It produced 195 alignments covering 2,076,916 query bases. Faqt covered all of
those bases, and 2,077,031 query bases in total. This checks **coverage only**:
it does not establish placement equivalence, equal sensitivity, or equal
precision. The presets, seed parameters, score thresholds and repeat policies
also differ. In particular, faqt retains alternative paths and subplacements
that minimap2 can filter or combine into larger alignments. A broader benchmark
is needed before claiming minimap2-equivalent alignment quality.

To rerun the whole-input gap regression, operation replay and symmetry check:

```sh
FAQT_SEQHIKER_DEBUG="/path/to/Seqhiker_debug" \
  go test ./align -run TestSeqhikerDebugPair -v -count=1
```

The normal suite skips this test unless that environment variable is set. The
engine and normal tests do not need minimap2 installed.

## Repeat-rich benchmark

See [the full benchmark report](../benchmarks/assembly/RESULTS.md) for repeated
measurements against minimap2 on both this pair and Shigella. The Shigella pair
exceeds the default raw-anchor budget; the report records that failure separately
from successful runs with a larger budget and compares repeat placements.

## SIMD and wavefront follow-up

See [the kernel study](../benchmarks/assembly/KERNELS.md) for optional ARM64 NEON
DP, scalar unrolling and wavefront experiments. NEON preserves byte-identical
whole-input alignments on both fixture pairs while retaining full end extension.

The [indexed-containment and sixteen-lane follow-up](../benchmarks/assembly/OPTIMIZATION.md)
compares each optimization independently and together, retaining the exact same
whole-input paths with full extension enabled.

## amd64 native validation

Native Linux/amd64 tests passed on a Ryzen 5 7600X on 2026-10-23 using
`GOAMD64=v1`, Go 1.27.2 and `GOEXPERIMENT=simd`. Scalar and AVX2 unit tests,
the original missing-region/input-swap regression, scalar fallback with AVX2
masked off, and AVX2 tests with core AVX-512 features masked off all passed.
All twelve full-input benchmark outputs matched the validated PAF hashes.
Median wall times improved by 3.50× on the original pair and 3.70× on repeats.
See [native results](../benchmarks/remote/RESULTS-2026-10-23.md) for measurements
and the [remote bundle](../benchmarks/remote/README.md) to reproduce the checks.

### Windows amd64

Native scalar and AVX2 unit suites passed on Windows 11 on the Ryzen 5 7600X.
The corrected Windows run also passed the original FASTA regression in both
backends, including missing-region recovery, operation replay and input-swap
symmetry (7,116 matches). The initial wrapper's CPU-mask check
failed because this Go toolchain does not apply GODEBUG CPU masks on Windows;
the corrected wrapper marks those checks unsupported. See the
[Windows validation record](../benchmarks/remote/WINDOWS-2026-10-23.md).

## Adaptive end-extension follow-up

The opt-in adaptive policy passes portable/SIMD suites, boundary-score equivalence,
race checks and the original FASTA replay/input-swap regression on Apple ARM64.
Three timed runs per mode/pair preserve repeatability; default full PAF hashes
are unchanged. Adaptive retains 99.97109% / 99.98452% of reference paired positions
on the original/repeat-rich fixtures. Native Linux/amd64 adaptive validation
also passed on the Ryzen 5 7600X on 2026-11-02: AVX2 and scalar unit tests,
full/adaptive original-genome replay and input-swap regressions, scalar fallback
with AVX2 disabled, and AVX2 with AVX-512 disabled. See the
[Linux adaptive validation record](../benchmarks/remote/ADAPTIVE-2026-11-02.md)
and [selection study](../benchmarks/assembly/ADAPTIVE-STUDY.md).
The adaptive changes have not been run natively on Windows; the earlier Windows
validation covers full mode before those changes.
