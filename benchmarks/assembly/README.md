# Assembly comparison benchmark

These opt-in tools compare faqt with an installed minimap2. They are development
benchmarks, not application dependencies. The Go engine remains pure Go and does
not require minimap2 or Python.

Run from the faqt repository, using Python 3 on macOS or Linux:

```sh
python3 benchmarks/assembly/run.py \
  --debug-dir "$HOME/Work/Seqhiker_debug" \
  --minimap2 /path/to/minimap2 \
  --out /tmp/faqt-assembly-benchmark --runs 3
python3 benchmarks/assembly/analyze.py /tmp/faqt-assembly-benchmark
python3 benchmarks/assembly/verify.py /tmp/faqt-assembly-benchmark
python3 -m unittest discover -s benchmarks/assembly -p 'test_*.py'
```

The script builds a Go test executable before timing anything. `--faqt-bin` can
select an already compiled benchmark test executable. Input FASTA files and raw
benchmark outputs are not bundled with faqt. The directory must contain the
original Seqhiker pair and/or `repeats.CP026802.1.fa` and
`repeats.CP151312.1.fa`; select pairs with repeated `--dataset` arguments.

Each sample runs in a fresh process. Measurements are sequential, with a warm OS
file cache, and include file reading, indexing/preparation, comparison and PAF
output. Go preparation/alignment/output durations are also recorded separately.
`GOMAXPROCS=1` limits Go execution to one logical processor. Minimap2 uses `-t 1`
except for the explicit three-worker baseline. Its I/O pipeline may still create
additional threads. CPU seconds and peak RSS are collected using `wait4` for each
child. Bidirectional minimap2 measurements sum two sequential process times;
their reported peak RSS is the maximum of the two, not their sum. All successful
samples contribute to the median, range and maximum observed RSS.

## Profiles

| Profile | Search | Settings |
|---|---|---|
| `faqt-default` | Both directions internally | `align.DefaultOptions()` |
| `faqt-raised` | Both directions internally | Defaults with anchor budget raised to 20 million |
| `faqt-full` | Both directions internally | Full end DP, anchor budget 20M on both pairs |
| `faqt-adaptive` | Both directions internally | Adaptive end DP, matching 20M anchor budget |
| `mm-asm5-t3` | B target, A query | `-x asm5 -c --eqx -P -t 3` |
| `mm-asm5-t1` | B target, A query | Same, one compute worker |
| `mm-asm5-both` | Both orders, sequentially | Same, one compute worker per order |
| `mm-similar-both` | Both orders, sequentially | Closer seeds, scoring and repeat retention; see below |

The closer minimap2 profile adds:

```text
-k 15 -w 10 -A 2 -B 4 -O 4,24 -E 2,1
-g 2000 -r 2000,2000 -z 200,200 -m 40 -s 40 -n 1
-f 100000000 -U 100000000,100000000 --q-occ-frac 0
--no-long-join --rmq=no
```

The [minimap2 2.31 manual](https://github.com/lh3/minimap2/blob/v2.31/minimap2.1)
documents that `-P` retains all surviving chains and disables primary/secondary selection;
`-N` and `-p` would have no effect. The closer profile disables practical
minimizer-frequency filtering on these inputs. Minimap2's remaining chaining,
backtracking, extension and output heuristics still differ from faqt's. Neither
configuration guarantees exhaustive enumeration of every homologous placement.
The same seed parameters do not imply identical seed sets or algorithms.

The raised-budget faqt profile runs only on Shigella; it would duplicate defaults
on the non-limiting original pair. The engine itself is not changed. Failures
are recorded explicitly, and subsequent repetitions of a failed profile are
skipped. The default process timeout is 300 seconds per invocation; change it
with `--timeout`. Timed-out or budget-exhausted comparisons have no complete
output and must not be assigned a hit count of zero.

## Comparing results

`analyze.py` normalizes both input orders onto genome A forward coordinates,
including reverse-strand CIGAR order and indel direction. It applies common
reporting filters to every engine: minimum span on each sequence 100 bases,
identity at least 70%, and final score at least 40 under faqt's scoring. These
filters cannot recover alignments an engine discarded during generation.

The analysis reports raw records, qualifying records, distinct paths, distinct
endpoint placements, coverage on both genomes, and coverage of **aligned
base-pair placements**. The latter is a union of intervals keyed by both contigs,
strand and alignment diagonal. It distinguishes one query base aligned to
several target positions, and removes overlap from repeated/subset records.
Paired positions include exact matches and substitutions; gaps do not contribute
paired positions. Identical endpoints can still have different internal paths.

Cross-engine intersection measures agreement in aligned coordinate pairs, not
accuracy against ground truth. Counts and multi-placement coverage can include
alternative gap placements as well as biological repeat copies. They therefore
must not be interpreted as repeat-copy counts or evidence of precision.

`metadata.json`, `timings.json`, per-process logs, PAFs, Go summaries and the
analyzed `summary.json` preserve the commands and measurements. Compare only
complete profiles. A wide runtime range or external machine load warrants
another run rather than treating one sample as definitive.

`verify.py` independently replays faqt PAF operations against the input DNA,
checks coordinate consumption and identity, and verifies byte-identical PAFs
across repeated faqt runs. It also rejects input files changed since timing.

## Separate diagnostic profile

To profile the first 20 seconds without mixing profiling into timed samples:

```sh
CGO_ENABLED=0 go test -c -o /tmp/assembly-profile.test ./benchmarks/assembly
GOMAXPROCS=1 \
FAQT_BENCH_A="$HOME/Work/Seqhiker_debug/repeats.CP026802.1.fa" \
FAQT_BENCH_B="$HOME/Work/Seqhiker_debug/repeats.CP151312.1.fa" \
FAQT_BENCH_OUTPUT=/tmp/shigella-profile \
FAQT_MAX_ANCHORS=20000000 FAQT_BENCH_TIMEOUT=20s \
FAQT_CPU_PROFILE=/tmp/shigella.cpu \
  /tmp/assembly-profile.test -test.run '^TestComparison$'
go tool pprof -top /tmp/shigella.cpu
```

This diagnostic intentionally exits nonzero at the context deadline. Its output
is incomplete and is not a timing or placement-comparison sample. The timeout
and profiling environment variables are optional; the timing runner uses the
default ten-minute internal deadline and its own process timeout. Diagnostic
profiles represent the sampled phase, not necessarily the whole run.

## DP kernel study

See [the scalar/NEON/WFA results](KERNELS.md) and the separate
[kernel harness](../kernels/README.md) for reproducible implementation-level
experiments. These keep full end extension enabled.

The [indexed-containment and sixteen-lane results](OPTIMIZATION.md) isolate
the later index and DP improvements. The kernel harness supports `wide` and
`--scan-containment` for reproducing those component comparisons.

## Adaptive end comparison

See [the adaptive selection study](ADAPTIVE-STUDY.md). Select `faqt-full` and
`faqt-adaptive` together for matching anchor budgets on both pairs; minimap2 is
not required when only faqt profiles are selected. The runner explicitly sets
end policy for every faqt profile, overriding an inherited benchmark policy.
Adaptive mode changes output; `analyze.py` measures paired-coordinate agreement
and `verify.py` independently replays each profile's PAFs against the DNA.
