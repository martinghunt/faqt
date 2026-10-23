# K-mer/window tuning — 2026-10-23

Exploratory single-threaded runs on Apple M4 Pro, Go 1.27.2, SIMD enabled. Each setting has one fresh-process timing; profiles were collected in separate baseline runs. All settings retain full DP end extension, the same scoring and filters, and the 20M benchmark anchor cap. No production default was changed. Default sketch is K=15, W=10.

Comparison time excludes preparation and PAF serialization. Retention measures the fraction of baseline aligned coordinate pairs present in the candidate, including contig pair, strand and alignment diagonal. This is reference agreement, not ground-truth accuracy. Different equally scoring gap paths can also affect agreement.

| K | W | Original seconds | Repeats seconds | Original pair retention | Repeats pair retention |
| ---: | ---: | ---: | ---: | ---: | ---: |
| 15 | 10 | 5.468 | 24.599 | 100.00000% | 100.00000% |
| 15 | 20 | 3.954 | 22.840 | 93.63296% | 99.79146% |
| 15 | 30 | 2.964 | 22.739 | 89.24568% | 99.63645% |
| 15 | 50 | 2.149 | 21.767 | 85.69312% | 99.53306% |
| 19 | 10 | 4.190 | 26.168 | 94.20182% | 99.66203% |
| 19 | 20 | 2.766 | 24.655 | 88.19472% | 99.55200% |
| 21 | 20 | 2.486 | 24.148 | 87.02308% | 99.46936% |

## Output checks

All fourteen PAFs passed independent replay against the input DNA. Both baseline PAF hashes exactly match the prior validated reference. Every original-pair setting retains the motivating missing-region match (same interval/identity condition as the original regression). Every original-pair setting also retains all baseline one-dimensional A/B coverage, despite losing alternative coordinate-pair placements. Counts and one-dimensional coverage therefore cannot establish equivalent placement sensitivity.

On repeats, W=20 loses 1,454 baseline A-covered bases and 1,089 B-covered bases; W=30 loses 1,790 and 1,277; W=50 loses 3,430 and 1,586. Raw coverage metrics, output hashes and replay counts are in [seed-tuning-results-2026-10-23.json](seed-tuning-results-2026-10-23.json). The MissingRegionRecovered field is inapplicable to the repeat dataset.

## CPU profiles and interpretation

Separate baseline CPU profiles place 76.35% of original-pair sampled CPU and 73.41% of repeat-pair sampled CPU under alignEnd. Total alignDPDifference cumulative shares are 82.69% and 80.76%, including internal gaps. Seed tuning reduces some extension candidates as well as seed lookup/sorting; its original-pair speedup must not be attributed solely to faster seed lookup. Increasing K to 19 at W=10 did not speed up the repeat case.

Wider windows reduce the original-pair runtime substantially, but recover fewer baseline placements; they offer only a modest repeat-rich speedup. These measurements support an optional faster sketch setting, not a claim that a coarser sketch preserves sensitivity. Keep K=15/W=10 as the sensitive default. To preserve current output, the main performance target remains end-extension DP, followed by parallel execution of independent comparisons. Neither prospective change is implemented or benchmarked by this experiment.

## Reproduction

Both genomes must use the same public align.SketchOptions{K: k, W: w}; preparation/sketching must be redone when changing either value. Compare with the same align.Options, EndExtensionDP and MaxAnchors=20000000. The isolated benchmark, driver, PAFs, profiles and source hashes remain under /private/tmp/faqt-seed-tuning/. The benchmark source differs only by exposing K/W environment inputs and recording the missing-region condition. It does not change the alignment engine. No private FASTAs or PAFs are added to the repository.
