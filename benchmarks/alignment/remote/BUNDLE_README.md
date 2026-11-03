# Linux amd64 alignment validation

This bundle contains static Linux/x86-64 binaries. It needs no Go, Python,
Codex, compiler, cgo library or network connection. A POSIX shell runs `run.sh`.
Minimap2 is optional: when installed, its single-worker comparison is included.

Extract the archive, enter its directory, then run:

```sh
./run.sh --runs 1 /path/to/Seqhiker_debug
```

Use `--runs 3` (the default) for repeated measurements. Put flags before the
positional directory. The directory can contain either or both complete pairs:

- `31585_6#49.fasta` and `barcode10_ICESp23_spliced.fasta`
- `repeats.CP026802.1.fa` and `repeats.CP151312.1.fa`

Copy the original files without rewriting them: their SHA-256 hashes are
checked. They are not included in this bundle. The job may take several minutes,
particularly for the scalar repeat-rich comparison. Avoid other CPU-heavy jobs
while timing. All alignment jobs run serially with `GOMAXPROCS=1`; minimap2 uses
`-t 1` and executes the two input orders sequentially. Minimap2 may also create
I/O pipeline threads. The closer seed/scoring/repeat settings from the previous
alignment benchmark are used.

For CPU/kernel checks before copying genome files:

```sh
./run.sh --self-test-only
```

The runner checks that the SIMD binary selects `avx2-16`, runs scalar and SIMD
unit tests, validates the scalar fallback with AVX2 masked off, and tests the
AVX2 path with the core AVX-512 features masked off. Full-data runs also replay
the original missing-region/input-swap regression and compare every scalar/SIMD
PAF to the previously validated reference hash. A mismatch marks the run failed
and keeps diagnostics. The runner refuses to label a scalar fallback as an
AVX2 benchmark.

On success or failure it prints `RESULT ARCHIVE: ...tar.gz`. Bring that compact
archive back for analysis. It contains CPU information, test logs, result JSON,
binary/input hashes, timings and optional profiles. Input FASTAs and PAFs are
excluded from the return archive; PAFs remain in the local results directory.
No results are uploaded automatically. Native correctness and speed on a new
target CPU remain unverified until its checks have run. Recorded Ryzen results
are linked from the repository's benchmarks/alignment/remote/README.md.

Useful options:

```sh
./run.sh --runs 3 --minimap2 /path/to/minimap2 /path/to/Seqhiker_debug
./run.sh --runs 1 --minimap2 none /path/to/Seqhiker_debug
./run.sh --runs 3 --profile /path/to/Seqhiker_debug
./run.sh --self-test-only --out /path/to/fresh-results-directory
```

`--profile` adds an extra untimed SIMD run per dataset and includes its CPU
profile in the return archive. `--timeout 15m` sets the per-test/benchmark
process deadline. Pressing Ctrl-C stops the active child process group and
attempts to save the partial diagnostics.

The binaries use `GOAMD64=v1` with runtime AVX2 detection. AVX2 arithmetic is
currently sixteen 8-bit lanes (128-bit vectors); this is the first amd64 port
of the shared ARM64 algorithm, not a 32-lane or AVX-512 implementation. Full
extension and the 20M benchmark anchor cap are retained. Algorithmic limits and
the normal library's 2M default anchor cap are unchanged.
