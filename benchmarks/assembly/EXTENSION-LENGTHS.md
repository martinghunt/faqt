# Observed end-extension lengths — 2026-10-23

Temporary instrumentation of the current full-extension SIMD engine records the
two extension results attached to each retained Match. The reference PAF bytes
remain unchanged on both complete fixture pairs. No instrumentation was added to
production code. K=15, W=10, Extension=256, full internal-gap and end DP, anchor
budget 20M. Length is max(query bases consumed, target bases consumed) per end;
it is not the number of exact matches or an overlap-deduplicated base count.

| Statistic, per retained alignment end | Original | Repeats |
| --- | ---: | ---: |
| Mean including zero ends | 22.83 bp | 10.26 bp |
| Median including zero ends | 5 bp | 0 bp |
| Zero-length ends | 44.01% | 85.54% |
| Mean among nonzero ends | 40.77 bp | 70.99 bp |
| Median among nonzero ends | 21 bp | 15 bp |
| 90th percentile, all ends | 55 bp | 10 bp |
| Ends longer than 64 bases | 8.75% | 4.27% |
| Ends reaching the 256-base cap | 1.99% | 2.31% |
| Nonzero ends with indels | 24.57% | 40.34% |

The samples contain 14,232 ends from 7,116 original-pair paths and 84,828 ends
from 42,414 repeat-pair paths. These describe retained output, not every DP
attempt or every distinct extension-cache entry. The diagnostic build adds
fields and output work, so its runtimes should not be used as new benchmarks.
Several candidate cores can produce the same final path; deduplication retains
one candidate's extension provenance. Histograms describe those retained cores.

Exact seed runs are already maximally extended before end DP. Thus a long exact
continuation immediately adjacent to the terminal anchor is not waiting for DP.
DP can cross mismatches and indels beyond that run. A long total gapped extension
does not require a long uninterrupted exact sequence. A matching minimizer is
also a chaining candidate, not a guarantee that every seed is included in each
selected chain. It remains worth auditing long extensions for reusable anchors
or duplicate work; this measurement does not prove that every long tail lacked
usable seeds. Among nonzero tails, 8.27% on the original pair and 26.57% on repeats
contain an exact operation of at least 24 bases, so one should not assume all
tails are seed-free. Both datasets have a small tail censored at 256 bases.

The cost comes from searching for a profitable extension, including failed
searches; it is not proportional to the length ultimately returned. This explains
how short or zero-length retained extensions can coexist with expensive DP.

See [extension-cap ablation](EXTENSION-SIZE.md) for final-output sensitivity and
runtime effects. Raw aggregate data: [extension-length-results-2026-10-23.json](extension-length-results-2026-10-23.json).
Diagnostic sources and per-end measurements were kept locally; the linked
aggregate data preserves the reported findings.
