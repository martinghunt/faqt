# Remote Linux/amd64 validation bundle

Build the AVX2 port and its native validation package on the development machine:

```sh
python3 benchmarks/alignment/remote/build_bundle.py --out /tmp/faqt-linux-amd64
```

This uses the installed Go toolchain (tested with Go 1.27.2), cross-compiles
`CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOAMD64=v1` binaries, and enables
`GOEXPERIMENT=simd` only in the candidate binaries. The output directory and its
`.tar.gz` archive must be fresh and outside the repository. No private input
sequences are packaged. `reference.json` contains input/output hashes and counts;
`build.json` records source hashes, compiler version and build commands.

The [bundle instructions](BUNDLE_README.md) explain native execution and returning
diagnostics. The runner itself requires no Python or Go installation on Linux.
Its tests cover successful command execution, partial failure archives, wrong
alignment/input hashes, per-process timeouts, and aggregation of sequential
minimap2 input orders. Platform-specific resource accounting supports Linux and
Darwin; the packaged binaries target Linux/amd64.

The shared sixteen-lane DP recurrence is tested natively on ARM64. The amd64
backend adds AVX2 dispatch and x86 widening/shuffle helpers, with scalar fallback.
Cross-compilation and local fallback tests do not establish native AVX2
correctness or speed: the generated package runs those checks on the target CPU
before measuring performance. Keep native-validation status explicit when
reporting results.

For correctness checks including both full and adaptive original-genome
regressions, without the timing benchmark:

```sh
./run.sh --self-test-only --data /path/to/Seqhiker_debug
```

Native Ryzen results: [full-mode measurements](RESULTS-2026-10-23.md) and
[adaptive correctness checks](ADAPTIVE-2026-11-02.md). Earlier native
[Windows validation](WINDOWS-2026-10-23.md) covers full mode.

Saved result snapshots replace machine-specific paths with generic placeholders;
timings, input/output hashes and source fingerprints are unchanged.
