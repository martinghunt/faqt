# Alignment benchmark

Compare faqt full/adaptive alignment with minimap2 using Python 3 on macOS or
Linux. These tools are optional; the library needs neither Python nor minimap2.

```sh
GOEXPERIMENT=simd python3 benchmarks/alignment/run.py \
  --debug-dir /path/to/Seqhiker_debug --minimap2 /path/to/minimap2 \
  --out /tmp/faqt-alignment-benchmark --runs 3
python3 benchmarks/alignment/analyze.py /tmp/faqt-alignment-benchmark
python3 benchmarks/alignment/verify.py /tmp/faqt-alignment-benchmark
```

SIMD needs a supporting toolchain (tested with Go 1.27.2). Omit
`GOEXPERIMENT=simd` for scalar execution. The runner builds the test executable;
`--faqt-bin` selects a prebuilt one. Inputs are not committed. The input directory
must contain the original pair (`31585_6#49.fasta` and
`barcode10_ICESp23_spliced.fasta`) and the repeat pair (`repeats.CP026802.1.fa`
and `repeats.CP151312.1.fa`), or select `--dataset debug` or `--dataset shigella`.

The default profiles are `faqt-full`, `faqt-adaptive`, and `mm-similar-both`.
Select profiles with repeated `--profile` arguments; minimap2 is unnecessary
when only faqt profiles are selected. Both faqt profiles use K=15/W=10, default
scoring and a 20M anchor budget, configurable with `--raised-anchors`. The
library's default 2M budget is insufficient for the repeat fixture; exceeding a
budget produces an error rather than a truncated alignment set.

Each sample runs in a fresh process, serially with `GOMAXPROCS=1`. Process wall
time includes input loading, preparation, alignment and PAF output. Comparison
time is also recorded separately. Minimap2 uses one compute worker (`-t 1`)
and both input orders sequentially; it may also create I/O threads. Reported
minimap2 time sums the two processes, and peak RSS is their maximum.

The minimap2 command uses `-x asm5 -c --eqx -P -t 1` plus:

```text
-k 15 -w 10 -A 2 -B 4 -O 4,24 -E 2,1
-g 2000 -r 2000,2000 -z 200,200 -m 40 -s 40 -n 1
-f 100000000 -U 100000000,100000000 --q-occ-frac 0
--no-long-join --rmq=no
```

These settings make scoring, seeds and repeat retention closer; the algorithms
and outputs still differ. Neither engine guarantees every homologous placement.

`analyze.py` normalizes input orders and applies common reporting filters:
100-base minimum span on each axis, 70% identity and score 40. It reports paths,
coverage, and agreement in aligned coordinate pairs, distinguishing alternative
repeat placements. Agreement is not accuracy against ground truth, and path
counts are not biological repeat-copy counts. `verify.py` replays faqt paths
against DNA, checks input hashes, and requires repeated PAFs to be identical.
Failures and timeouts are incomplete results, not zero-hit comparisons.

The output directory contains metadata, process logs, PAFs, timings, analysis
and verification JSON. A timeout defaults to 300 seconds per process and can be
changed with `--timeout`. Saved repository snapshots use generic paths.

See [results and limitations](RESULTS.md) and [native validation](remote/README.md).
Run the harness tests with:

```sh
python3 -m unittest discover -s benchmarks/alignment -p 'test_*.py'
```
