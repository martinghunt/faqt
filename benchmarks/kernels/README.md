# Alignment kernel experiments

This opt-in harness compares the original scalar DP loop, an unrolled scalar
loop, ARM64 NEON DP (four-lane and sixteen-lane), a Go dual-affine wavefront prototype, a bounded-WFA/NEON
hybrid, and optionally the reference WFA2 C implementation. It builds an isolated
copy of faqt; it does not add WFA or cgo to the normal library.

The SIMD build needs a Go toolchain supporting `GOEXPERIMENT=simd` (tested with
Go 1.27.2). SIMD measurements require an ARM64 NEON or amd64 AVX2 host. On
amd64, runtime detection selects scalar fallback if AVX2 is unavailable.
Other architectures use scalar fallback, so those timings are not SIMD measurements.

From the faqt repository:

```sh
python3 benchmarks/kernels/build.py --out /tmp/faqt-kernels
python3 benchmarks/kernels/run.py --study /tmp/faqt-kernels \
  --debug-dir "$HOME/Work/Seqhiker_debug" --dataset debug \
  --kernel scalar --kernel fast --kernel simd --runs 3 --capture
python3 benchmarks/kernels/run.py --study /tmp/faqt-kernels \
  --debug-dir "$HOME/Work/Seqhiker_debug" --dataset repeats \
  --kernel scalar --kernel fast --kernel simd --runs 3 --capture
```

`--out` must be a fresh directory outside the repository. Runs are serial and
use `GOMAXPROCS=1`, full DP end extension, and a 20M anchor cap. Process wall/CPU,
peak RSS, Go allocations and output hashes are written alongside PAFs. The Go
allocation counter excludes C allocations. Python timing includes process
startup; `AlignSeconds` measures comparison alone. Compilation is excluded.

The `scalar` mode is the pre-optimization reference retained in
`assets/dp_reference.go.txt`. `fast` is the current portable scalar kernel.
`simd` selects the current architecture kernel: sixteen-lane score-difference
DP where supported, with the four-lane (ARM64) or scalar fallback elsewhere. `wide`
selects the earlier four-lane full-score NEON kernel on ARM64. Containment
checks use the current indexed implementation in all modes unless the source
copy was built with `--scan-containment`. Thus `scalar` is a DP reference, not
a snapshot of the entire historical pipeline. `wfa` tries the Go WFA prototype with a
512-cost-layer budget before falling back to scalar DP. `hybrid` tries 64 cost
layers before falling back to NEON DP. These cost budgets are transformed WFA
costs, not mismatch counts. WFA searches a fixed diagonal band and computes the
best anchored prefix (extension) or fixed-endpoint alignment (gap). It does not
reproduce the scalar kernel's row-order Z-drop. Traceback ties can also differ.
The WFA prototype stores traceback wavefronts explicitly; its allocation cost
is a substantial limitation, not a lower bound for WFA implementations.

To include the optimized native reference, build
[WFA2](https://github.com/Marco-Sola-Lab/WFA2-lib) with
`make -j4 BUILD_EXAMPLES=0 BUILD_TOOLS=0`, then supply its source path:

```sh
python3 benchmarks/kernels/build.py --out /tmp/faqt-kernels-native \
  --wfa2 /path/to/WFA2-lib
python3 benchmarks/kernels/run.py --study /tmp/faqt-kernels-native \
  --debug-dir "$HOME/Work/Seqhiker_debug" --dataset debug \
  --kernel wfa --kernel hybrid --kernel wfa2
```

The adapter uses dual-affine scoring identical to faqt defaults, static banding,
full CIGAR traceback and reusable native aligners. Its extension mode is WFA2's
anchored extension with maximum-score CIGAR trimming. That mode is measured as
a separate candidate: it is not assumed equivalent to faqt's best-prefix
extension or row-order Z-drop. Custom scoring falls back to scalar. Resource
limits and cancellation inside a native call are not implemented by this
benchmark-only adapter. Production has no native adapter.

Capture retains every seventeenth reference call, up to 1,800 extensions and
1,800 internal gaps per dataset. It samples early calls rather than a uniform
sample of the entire comparison. Original sequences and reference operations
are included in the private JSONL; do not commit those files. To replay the
captured workloads and validate kernels:

```sh
FAQT_SAMPLES=/tmp/faqt-kernels/results/debug.samples.jsonl \
FAQT_SAMPLE_RESULT=/tmp/faqt-kernels/results/debug.samples.json \
GOMAXPROCS=1 /tmp/faqt-kernels/align.test \
  -test.run '^TestStudySamples$' -test.v
/tmp/faqt-kernels/align.test \
  -test.run '^Test(KernelStudy|WFAStudy)$' -test.v
GOMAXPROCS=1 /tmp/faqt-kernels/align.test -test.run '^$' \
  -test.bench '^BenchmarkKernelStudy$' -test.benchmem
```

Captured-call tests require scalar and NEON outputs to match exactly. They
report WFA/WFA2 score, endpoint and traceback differences separately. The Go WFA
is also checked against scalar DP with Z-drop disabled and its operations
replayed against DNA. Whole-genome PAFs can be replayed using
`benchmarks/assembly/verify.py` helpers and compared with the interval-based
placement analysis in `benchmarks/assembly/analyze.py`.

Algorithm references: [WFA paper](https://pmc.ncbi.nlm.nih.gov/articles/PMC8355039/)
and [WFA2 library](https://github.com/Marco-Sola-Lab/WFA2-lib). The Go prototype
uses furthest-reaching wavefronts for both affine gap costs. Match rewards are
converted to nonnegative costs: before common-factor reduction, mismatch costs
`2*(Mismatch+Match)`, gap opens cost `2*GapOpen`, and extensions cost
`2*GapExtend+Match`. Endpoint score is recovered as
`(Match*(consumedQuery+consumedTarget)-transformedCost)/2`.

## Index and DP ablation

Build one source copy normally and another with the saved pre-index scans:

```sh
python3 benchmarks/kernels/build.py --out /tmp/faqt-indexed
python3 benchmarks/kernels/build.py --out /tmp/faqt-scans --scan-containment
```

For each dataset, run `--kernel wide` and `--kernel simd` on both copies using
`run.py` above. This produces four combinations: scans plus four-lane DP
(the previous implementation), indexed containment plus four-lane DP, scans
plus score-difference DP, and both optimizations. Use fresh output directories,
serial processes and `GOMAXPROCS=1`; the runner supplies that processor limit.
Compare PAF hashes as well as time. The current scalar reference and every
optimized combination should retain identical paths. The `.scan.txt` templates
are benchmark-only historical implementations and do not enter the library.

For Linux/amd64 native checks without installing Go, use the
[precompiled remote bundle](../remote/README.md). Native AVX2 validation is
pending until its results have been collected from an AVX2 machine.
