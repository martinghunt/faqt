# Alignment rewrite handoff — 2026-10-23

## Current state

This Git checkpoint captures the assembly-alignment optimisation work.
An opt-in adaptive end-extension mode is now implemented (details
at the end of this file). It is not integrated into Seqhiker. Seqhiker still uses its existing
zem/comparison.go engine; its pre-existing uncommitted comparison.go and
comparison_test.go edits must be preserved. The faqt CLI does not yet expose
the align package either. No production defaults or build scripts were changed
by the latest tuning experiments.

The motivating case is 31585_6#49.fasta versus
barcode10_ICESp23_spliced.fasta in ~/Work/Seqhiker_debug: the old application
missed a match in .31585_6_49.4 between long matches. The new full-input
regression recovers the region around 66,800–69,700, replays paths against DNA,
and checks exact input-swap symmetry. The repeat-rich fixtures are
repeats.CP026802.1.fa and repeats.CP151312.1.fa in the same directory.

## Implemented

- Reusable per-genome minimizer indexes, maximal exact-run compaction,
  repeat-preserving chaining, standalone exact-run candidates, full internal
  gap DP and outward end extension, and deterministic symmetric results.
- Indexed exact-subpath containment in place of repeated full-list scans.
- Pooled sixteen-lane Suzuki–Kasahara score-difference DP with two-piece affine
  gaps, exact scalar traceback/tie rules and scalar row-order Z-drop semantics.
- Shared SIMD algorithm: ARM64 NEON and runtime-guarded amd64 AVX2. Unsupported
  x86 CPUs use scalar DP; unsupported scoring/kernel sizes retain fallback.
  The current x86 implementation has sixteen 8-bit lanes (128-bit vectors),
  not a 32-lane AVX2 or AVX-512 implementation.
- Portable scalar optimisations, regression/fuzz/oracle tests, and remote native
  validation bundles. No cgo/native alignment dependency was introduced.

Production defaults: K=15, W=10; full end DP up to 256 bases per end;
BandWidth=96; MaxGap=2000; minimum span 100 and identity 70%; default anchor cap
2M. Repeat-rich benchmark runs raise the anchor cap to 20M. A resource-limit
failure is an error, not a complete alignment result.

## Build and distribution

Use the tested Go 1.27.2 toolchain and GOEXPERIMENT=simd to include SIMD code.
Plain builds use scalar DP. The module's go 1.25 directive by itself does not
select the SIMD-capable toolchain. Keep GOAMD64=v1 so the overall amd64 binary
remains baseline-compatible; runtime dispatch selects AVX2. ARM64 uses baseline
NEON. There is still one binary per OS/architecture, with no SIMD-specific
user download or runtime environment flag required.

Neither faqt/build.sh nor Seqhiker/build_zem_bins.sh currently enables SIMD by
default. They can inherit GOEXPERIMENT=simd from the calling environment.
When integrating into Seqhiker, ensure the actual consumer build enables SIMD
and uses the updated faqt dependency. Merely rebuilding today's Seqhiker with
that flag does not connect its old engine to faqt. The current faqt CLI does not
import align, so its own CLI binary does not yet exercise the new aligner.

## Validation completed

- Apple M4 Pro / macOS ARM64: normal and SIMD full Go suites, targeted vet,
  race checks, scalar/oracle/random comparisons, fuzzing, captured-call checks,
  original FASTA regression and exact full-pair PAF hashes.
- Ryzen 5 7600X / Linux amd64: native avx2-16 selection; scalar and SIMD suites;
  original regression; three timed runs of both engines on both pairs. Every
  output matches the validated reference. AVX2-disabled scalar fallback and
  AVX2 with core AVX-512 features masked off both pass.
- Same Ryzen / Windows 11 dual boot: native avx2-16 and scalar suites, original
  FASTA regression, replay and input-swap checks pass. Corrected wrapper passes.
  No repeat-rich Windows benchmark or Windows PAF-hash comparison was done.
- Other OS/architecture combinations were cross-compiled; do not describe that
  as native execution on all CPUs.

Windows wrinkle: this Go toolchain initializes CPU features before reading
GODEBUG on Windows, so GODEBUG=cpu.avx2=off does not force scalar there. The first
Windows wrapper falsely failed its backend expectation despite passing native
alignment tests. Corrected wrapper explicitly skips unsupported masking checks.
This was a validation-wrapper issue; no production alignment fix was required.
See [native Linux results](benchmarks/remote/RESULTS-2026-10-23.md) and
[Windows record](benchmarks/remote/WINDOWS-2026-10-23.md).

Full-extension reference PAF SHA-256:
- Original, 7,116 paths: e3f43bc6cf4d0fad26d32e03ffe02a79d017d8a22020efdafb9a1e20773a1270
- Repeats, 42,414 paths: c55b448e430ff594bb94b18a121f14e6a8cf82ccb7af0cb2c0744f65a16a65e4

## Speed: distinguish the baselines

The new engine is much faster than the earlier scalar faqt implementation, but
it is slower than Seqhiker's current initial-summary engine. Do not conflate them.

Direct comparison on M4 Pro, GOMAXPROCS=1, median comparison seconds across three
fresh processes, excluding loading/serialization:

| Pair | Existing Seqhiker initial summaries | Current faqt full paths | Outputs: old / new |
| --- | ---: | ---: | ---: |
| Original | 2.245 s | 5.537 s | 2,720 summaries / 7,116 paths |
| Repeats | 2.987 s | 24.646 s | 1,024 summaries / 42,414 paths |

The new initial comparison is 2.47× / 8.25× slower on these inputs, while doing
more work. Old Seqhiker limits seed occurrences to 64, refined blocks to 1,024
per segment pair and refinement work to an 80M-cell budget; detailed operations
are generated on demand. It uses different filters (including 50% minimum
identity) and redundancy/chaining rules. New faqt computes paths immediately.
Counts are not directly interchangeable biological-copy or accuracy measures.
The isolated benchmark/report is /private/tmp/seqhiker-vs-faqt/REPORT.md;
raw samples and source hashes are beside it. Seqhiker application source was
not changed by the measurement.

On the Ryzen/Linux, current scalar versus AVX2 median process wall seconds:
original 24.789 → 7.089 (3.50×); repeats 115.683 → 31.260 (3.70×).
Windows original regression logs 25.08 s scalar versus 7.08 s AVX2 for its first
comparison, a single measurement rather than a repeated wall-time benchmark.

M4 full-application-engine comparison versus minimap2 -t 1, matched repeat-
retaining settings and both input orders sequentially: faqt median wall
5.637 / 24.572 s versus minimap2 2.872 / 31.272 s. Engines produce different
paths. No same-machine Ryzen minimap2 measurements were returned.
See [optimisation study](benchmarks/assembly/OPTIMIZATION.md).

## Latest tuning findings — defaults unchanged

Seven K/W settings were tried with full extension. Wider windows speed up the
original pair but lose alternative aligned coordinate pairs. K=15/W=30 gives
2.964 / 22.739 s on original/repeats and retains 89.25% / 99.64% of baseline
paired positions. Larger K did not consistently help repeats. Original missing
region is recovered at every tried setting, but that alone does not establish
equivalent sensitivity. One-dimensional coverage also hides missing placements.
See [seed tuning](benchmarks/assembly/SEED-TUNING.md).

End-extension DP accounts for about 73–76% of sampled baseline CPU. Changing
only its per-end cap, keeping internal gap DP and K=15/W=10:

| End cap | Original seconds / reference pairs retained | Repeats seconds / reference pairs retained |
| --- | --- | --- |
| 0 | 0.673 / 88.417% | 3.947 / 99.448% |
| 16 | 0.881 / 91.830% | 4.857 / 99.722% |
| 32 | 1.114 / 95.895% | 5.841 / 99.822% |
| 64 | 1.641 / 98.883% | 8.075 / 99.906% |
| 128 | 2.921 / 99.779% | 13.512 / 99.945% |
| 256 | 5.504 / 100% | 24.536 / 100% |

These are exploratory single runs. A 64-base cap is a promising optional faster
mode, about 3× faster on both, but is not equivalent output and is not adopted
as the default. No-end extension still fills gaps between anchors. Versus no
extension, full end extension adds only 0/26 uniquely covered A/B bases on the
original pair and 2,250/1,183 on repeats; most changes concern alternative
placements and ends. All ablation PAFs replay correctly. See
[extension cap study](benchmarks/assembly/EXTENSION-SIZE.md).

The user's latest question was whether tails should be short because another
minimizer would already have been used. Direct retained-output instrumentation
now measures mean extension per end of 22.83 bp (original) and 10.26 bp (repeats),
including zeros; medians are 5 and 0. Zero-length fractions are 44.01% and 85.54%.
Among nonzero ends, means are 40.77 and 70.99 bp, medians 21 and 15. About 1.99%
and 2.31% of all ends hit the 256-base cap. Exact anchors have already been
maximally extended; DP can bridge mismatches/indels and a minimizer candidate
need not be selected in every chain. Some long tails do contain long exact
runs, so do not assume they all lacked usable seeds. Auditing these for reusable
anchors or duplicate work remains worthwhile. Searching failed tails costs CPU
even when the reported extension is zero. See
[actual extension lengths](benchmarks/assembly/EXTENSION-LENGTHS.md).

## Remaining work / next decisions

1. An opt-in adaptive mode is now implemented (see the current follow-up below).
   Validate it natively on x86 and integrate an explicit fast/full choice as
   appropriate; the sensitive full-DP default remains unchanged.
2. Profile/optimise end-extension work while retaining full-output tests;
   inspect long tails and repeated searches before assuming unavoidable cost.
   Parallel independent comparisons are another possibility; not implemented.
3. Integrate faqt into Seqhiker with progress, resource errors, segment
   coordinates, detail/variant presentation and regression coverage. Old summary
   latency versus new eager paths is a product decision, not just a kernel issue.
4. Make the consumer/release build enable SIMD reproducibly with the tested
   toolchain. Verify the actual application release artifacts after integration.
5. Broaden native hardware/data testing as needed; same-machine Ryzen minimap2
   comparison and Windows repeats remain unmeasured.

WFA was explored: Go prototype did not beat SIMD on these complete pairs;
native WFA2 extension changed endpoints/paths. It remains benchmark-only, with
no production WFA or cgo dependency. Details: benchmarks/assembly/KERNELS.md.

## Local artifacts

- Private FASTAs: ~/Work/Seqhiker_debug/ (never committed).
- Current native Linux bundle: /private/tmp/faqt-amd64/faqt-linux-amd64.tar.gz.
- Returned Linux results: ~/Work/results-76565828.tar.gz.
- Corrected Windows bundle: /private/tmp/faqt-windows-amd64-v2.zip.
- Returned Windows results: ~/tmp/results-14064-6966/ (initial wrapper failure),
  ~/tmp/results-15445-31558/ (corrected wrapper + FASTA regression pass).
- Earlier optimisation/WFA work: /private/tmp/faqt-opt2/, /private/tmp/faqt-kernel-study/.
- Recent isolated experiments: /private/tmp/seqhiker-vs-faqt/,
  /private/tmp/faqt-seed-tuning/, /private/tmp/faqt-extension-size/,
  /private/tmp/faqt-extension-lengths/.

Temporary diagnostic fields/harness changes exist only in those experiment
copies; do not copy them into production. Repository reports/JSON retain the
findings if temporary binaries or raw PAFs disappear. The experiments were uncommitted while measured; this checkpoint saves their results.

## Follow-up: adaptive 64-base growth prototype

The user's proposed 64 → 128 → 192 → 256 growth rule was measured in an isolated
benchmark copy. Grow only when the current best endpoint touches a genuinely
truncated input boundary; retries currently rerun from the original anchor.
Adaptive median comparison times are 1.737 s original and 9.405 s repeats,
versus fresh full-256 controls 5.569 / 24.689 s (one control sample, three adaptive
samples). It retains 99.14708% / 99.92108% of reference paired positions and
recovers the motivating match, but changes output. All PAFs replay correctly.
Only 2.49% / 4.06% of initial extension attempts request growth beyond 64.

A synthetic DP counterexample confirms that best-endpoint-before-boundary is
not a safe stop: best prefix is at 32 with score 58 in the 64-base matrix, but
the full 256-base matrix recovers through a mismatch valley to score 330.
An equivalent-output version needs boundary-state continuation and safe stopping
bounds / unchanged row-order Z-drop, not merely appending chunk alignments.
That initial prototype was not adopted; the selected mode below supersedes it. The prototype and raw artifacts
are /private/tmp/faqt-adaptive-extension/.

See [adaptive extension report](benchmarks/assembly/ADAPTIVE-EXTENSION.md).

## Current follow-up: selected adaptive mode implemented

The adaptive study screened 23 policies on the original pair and seven selected
policies on repeats, alongside full controls. An opt-in EndExtensionAdaptive is
now implemented in align. It starts at 64 bases, doubles as needed, and observes
best scores at the truncated input edges, continuing if they are within 24 match
rewards of the best prefix (48 points at defaults). Normal row-order Z-drop is
preserved. Each larger attempt recomputes from the original anchor. Internal gap
alignment, dense seeds, default maximum 256 and full-DP default remain unchanged.

Production validation: three fresh-process runs per pair/mode, GOMAXPROCS=1,
M4 Pro / Go 1.27.2 / SIMD / anchor20M. Median comparison seconds:

| Pair | Full | Adaptive | Reference pairs retained | Unique A/B bases lost |
| --- | ---: | ---: | ---: | ---: |
| Original | 5.605 | 3.147 | 99.97109% | 0 / 0 |
| Repeats | 24.971 | 10.898 | 99.98452% | 55 / 0 |

That is 1.78× / 2.29× faster. Full PAF hashes remain exactly unchanged. All
repetitions are byte-identical within mode/pair; every distinct output replays
against DNA. Both modes pass the motivating missing-region/input-swap regression.
Adaptive returns 7,116 / 42,333 paths; the same original count does not mean the
same paths. Retention is reference agreement, not a universal accuracy estimate.

Public use: opts := align.DefaultOptions(); opts.EndExtension = align.EndExtensionAdaptive.
Default EndExtensionDP remains available and unchanged. The adaptive rule can
miss later recovery across a score valley, including a documented synthetic
counterexample. It is not an equivalent-output replacement for full extension.

Portable/SIMD full suites, vet, race checks, 900 randomized scalar/platform
boundary comparisons, scoring scale, concurrency, internal-gap/repeat tests and
14 Python harness tests pass. Updated code cross-compiles for Linux/Windows
amd64, Linux/Windows ARM64, Darwin amd64 and portable Linux/386. Translated amd64
checks and explicitly disabled-AVX2 fallback pass; translation is not counted as
native AVX2 testing. New-mode native Ryzen/Linux and Windows execution remains
pending. Earlier native full-mode results predate the boundary-probing change.

Matching faqt-full and faqt-adaptive profiles are now available in the assembly
benchmark runner with 20M anchor budgets on both pairs. Go-only profiles require
no minimap2. The source uses per-call boundary pointers, no experiment globals or
runtime environment switches. Prototype settings remain benchmark-only.

Fresh native test bundle (no Go installation needed):
/private/tmp/faqt-adaptive-study/faqt-adaptive-native-checks.zip.
Linux: linux-amd64/run-tests.sh /path/to/Seqhiker_debug.
Windows: windows-amd64/run-tests.cmd "C:\path\to\Seqhiker_debug".
Return generated results directories. These are correctness checks, not repeated
native adaptive performance measurements. The original FASTAs are optional;
without their directory the real-data regressions explicitly skip.

Still not integrated into Seqhiker; build scripts still need SIMD-enabled consumer
build settings when integration happens. The experiments were uncommitted while measured; this checkpoint saves their results. The next work
is native x86 validation and Seqhiker integration, not more parameter searching
on the same two fixtures. Resumable DP could reduce retry work later, but is not
required for this first opt-in mode. New data would be more useful than further
tuning against the same fixtures.

See [adaptive selection report](benchmarks/assembly/ADAPTIVE-STUDY.md).

## Checkpoint comparison: all engines

Median total process seconds on the same Mac, with single-threaded settings.
The existing Seqhiker and minimap2 rows reuse the earlier measurements; they were
not rerun for this checkpoint. Minimap2 includes both input orders sequentially.

| Pair | Existing Seqhiker, missing hits | Full faqt | Adaptive faqt | minimap2 -t 1 |
| --- | ---: | ---: | ---: | ---: |
| Original | 2.34 | 5.83 | 3.35 | 2.87 |
| Repeats | 3.19 | 25.48 | 11.40 | 31.27 |

Existing Seqhiker returns summaries with lazy detail and caps repeat refinement;
faqt returns full paths eagerly. Algorithms and output sets differ. The original
Seqhiker comparison is preserved in
[SEQHIKER-COMPARISON.md](benchmarks/assembly/SEQHIKER-COMPARISON.md).
