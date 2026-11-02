# Assembly comparison benchmark — 2026-10-23

On these files, faqt is about **9.5× slower** than the closer minimap2 configuration
on the original pair and **5.1× slower** on Shigella after raising its anchor cap.
Faqt defaults fail on Shigella before producing results. The raised cap allows
completion with 42,414 paths, preserving almost all aligned coordinate pairs
found by the repeat-retaining minimap2 configuration.

## Method

Core faqt revision: `e01f31ab52bc67952a4993310f1ba52676cc8c8c`. Minimap2: `2.31-r1302`.
Compiler: `go version go1.27.2 darwin/arm64`. Apple M4 Pro; `macOS-26.7.1-arm64-arm-64bit-Mach-O`.

Three fresh-process samples per successful configuration, measured sequentially
with warm OS file caches. Wall time includes input reading, indexing/preparation,
comparison and PAF output. Compilation and the later placement analysis, replay
checks and profiling are excluded. Faqt uses `GOMAXPROCS=1`. The closer minimap2
comparison uses one compute worker per process and searches both input orders
sequentially, matching faqt’s bidirectional workflow. These are closer settings,
not identical algorithms. [Commands and definitions](README.md) document the
remaining differences.

| Pair | Genome A | Genome B |
|---|---|---|
| Original | 31585_6#49: 18 contigs, 2,084,040 bases | barcode10: 1 contig, 2,095,267 bases |
| Shigella | CP026802.1: 4,975,028 bases | CP151312.1: 4,866,302 bases |

## Comparable bidirectional results

Paths use common output filters: span ≥100 bases on each sequence, identity ≥70%,
and final score ≥40 under faqt’s scoring. Both orders are normalized onto A/B.
Exact duplicate paths are collapsed; distinct endpoints and internal paths can
still differ between tools. RSS is the maximum observed process peak across all
samples; for two sequential minimap2 processes it is their maximum, not sum.

| Pair | Engine | Median wall | Sample range | Median CPU | Peak RSS | Qualifying distinct paths |
|---|---|---:|---:|---:|---:|---:|
| Original pair | faqt defaults | 25.844 s | 25.742–26.028 s | 25.837 s | 275 MiB | 7,116 |
| Original pair | minimap2 closer settings, both orders | 2.728 s | 2.727–2.737 s | 2.694 s | 75 MiB | 4,928 |
| Shigella | faqt defaults | Failed | — | — | 550 MiB | Incomplete |
| Shigella | faqt, 20M anchor cap | 151.915 s | 150.948–152.066 s | 151.974 s | 1240 MiB | 42,414 |
| Shigella | minimap2 closer settings, both orders | 29.560 s | 29.463–29.745 s | 29.513 s | 567 MiB | 40,169 |

The Shigella default run exceeded **2,000,000 raw seed anchors per contig pair and
strand**, before exact-run compaction. It failed in 0.452 seconds with no complete
output. This is not zero homologous hits. Subsequent repetitions of that failed
configuration were skipped. Only `MaxAnchors` changes in the successful raised
configuration, from 2 million to 20 million; no engine code was changed for these
benchmarks. The non-limiting original pair did not need a raised-cap rerun.

Preparation was about 0.14 seconds on the original pair and 0.32 seconds on
Shigella. Comparison itself accounts for nearly all faqt wall time. Median
cumulative allocations during comparison were about 6.6 GiB and 34.8 GiB
respectively; those are allocation churn, not resident memory.

## Repeat-placement agreement

Each aligned coordinate pair is keyed by both contigs, strand and position on
both genomes. This avoids equating several overlapping ribbons with additional
repeat copies, and avoids treating one-dimensional coverage as proof that all
placements were found. Paired positions include exact matches and substitutions;
gaps do not create paired positions.

| Pair | faqt paired positions | Minimap2 paired positions | Shared | Minimap2 positions also in faqt | faqt positions also in minimap2 |
|---|---:|---:|---:|---:|---:|
| Original pair | 3,088,814 | 2,840,928 | 2,787,420 | 98.117% | 90.242% |
| Shigella | 36,338,861 | 35,978,478 | 35,932,062 | 99.871% | 98.881% |

On Shigella, faqt averages 7.62 distinct oriented target placements per paired A
base, versus 7.55 for the closer minimap2 configuration; about 648 kb and 638 kb
of A respectively have multiple placements. The corresponding means on the
original pair are 1.49 and 1.37. These figures include alternative gap placements
as well as biological repeats, and are not repeat-copy counts.

Faqt also includes 99.9955% of the original pair’s and 99.9985% of Shigella’s
aligned coordinate pairs from bidirectional stock `asm5`. Stock `asm5` retains
fewer repeat placements and uses different seeds, scoring and joining behavior.

All faqt paths were independently replayed against the original DNA: operations,
coordinate consumption and identities agreed. The original pair’s 7,116 paths
and Shigella’s 42,414 paths each produced byte-identical PAFs across all three
runs. Cross-engine agreement is not accuracy against ground truth. Missing or
additional coordinate pairs can reflect different local alignments or ambiguous
gap placements; no claim of exhaustive discovery or equal precision is made.

## Other minimap2 baselines

The earlier 1.16-second result is reproducible, but uses three workers, one input
order and stock `asm5` settings. It is not a controlled comparison with faqt’s
single-processor, bidirectional search.

| Pair | Minimap2 configuration | Median wall | Peak RSS | Qualifying distinct paths |
|---|---|---:|---:|---:|
| Original pair | minimap2 asm5, 3 workers, one order | 1.168 s | 722 MiB | 195 |
| Original pair | minimap2 asm5, 1 worker, one order | 2.343 s | 451 MiB | 195 |
| Original pair | minimap2 asm5, both orders | 4.807 s | 542 MiB | 257 |
| Shigella | minimap2 asm5, 3 workers, one order | 20.780 s | 1544 MiB | 9,564 |
| Shigella | minimap2 asm5, 1 worker, one order | 20.681 s | 1542 MiB | 9,564 |
| Shigella | minimap2 asm5, both orders | 39.454 s | 1543 MiB | 11,260 |

## Engineering implications

- Shigella belongs in the acceptance set: the current default raw-anchor budget
  prevents a required workload from running. Compaction before enforcing a
  meaningful work budget is preferable to permanently relying on a larger cap.
- End extension remains the main performance target. A separate, untimed CPU
  profile of the first 20 seconds of the raised-budget Shigella run attributed
  approximately 95% of sampled CPU to base DP, including about 85% to end
  extension. This samples the initial portion, not the entire comparison.
- Optimize work per retained placement and allocation churn while preserving
  repeat-placement coverage. Reducing the number of placements with top-N or
  frequency filters would change the product requirement.

## Reproduction and retained results

Use the commands in [README.md](README.md). The compact, machine-readable
[result snapshot](results-2026-10-23.json) records the measurements, placement
agreement, input hashes and verification. Raw logs, PAFs and diagnostic profiles
were kept locally; input sequences and large raw PAFs are not committed.
