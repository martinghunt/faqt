# Adaptive 64-base end-extension prototype — 2026-10-23

Benchmark-only implementation of the user's proposal: run end DP on the first
64 bases of each flank; if the best endpoint reaches a truncated input boundary,
retry from the original anchor on 128, then 192, then 256 bases. A boundary is
eligible to trigger growth only if the original flank has additional sequence.
Each retry reruns DP; this prototype does not append independent alignments or
reuse a saved frontier. Full scoring and internal-gap DP are unchanged. No
production code or default was changed.

Apple M4 Pro, Go 1.27.2, GOEXPERIMENT=simd, GOMAXPROCS=1; K=15/W=10,
MaxAnchors=20M. Three adaptive samples per pair, one fresh full-256 control.
Comparison seconds exclude preparation and output serialization.

| Pair | Full 256 control | Adaptive median | Speedup | Reference paired positions retained |
| --- | ---: | ---: | ---: | ---: |
| debug | 5.569 s | 1.737 s | 3.21× | 99.14708% |
| repeats | 24.689 s | 9.405 s | 2.63× | 99.92108% |

Every output passed independent DNA replay. Adaptive PAFs are byte-identical
across their three repeats; full controls match the previous validated reference
hashes. The original missing-region condition remains satisfied. Retention is
agreement with reference paired coordinates, not a ground-truth accuracy score.
The adaptive original-pair result retains all baseline one-dimensional coverage;
the repeat result loses 1,083 A-covered bases and 350 B-covered bases.

Attempts at caps 64/128/192/256 are 39,191/976/297/184 on the original pair and
168,647/6,842/5,411/4,968 on repeats. Only 2.49% / 4.06% of initial attempts
request a second stage. These are DP requests after the current refinement
cache, including candidates later filtered or deduplicated, not retained ends.
Repeated stages still have extra cost; storing all required DP boundary state
could avoid recomputation, but is not implemented here.

## Why banding does not remove the opportunity

The current band is |i-j|<=96 and already stops on row-order Z-drop. Before any
early drop, a 64-by-64 input square has 4,225 cells including the origin, whereas
a 256-by-256 input square with that band has 40,289. The band limits diagonal
drift; it does not limit how far along the sequences DP searches. Actual visited
cell counts can be lower because of Z-drop.

## Why the stopping rule is heuristic

The best prefix can stop inside the first 64 bases while a lower-scoring path
at the boundary later recovers and beats it. The DP boundary contains alternative
scores/gap states, not just the selected prefix endpoint. This occurs in the
actual fixture comparison, as the reference-pair retention figures demonstrate.

A targeted synthetic check also passes in the isolated source:
TestAdaptiveBoundaryCounterexample has its best 64-base result at q=t=32 with
score 58, but the 256-base result reaches q=t=256 with score 330 after crossing
a mismatch valley. Therefore `best endpoint < 64` cannot certify that growing
the matrix would be useless. This test illustrates the DP stopping limitation;
it is not a realistic benchmark of whole-genome seed/chaining behavior.

For an equivalent-output adaptive kernel, retain the full required boundary DP
states, traceback, global best and scalar row-order Z-drop bookkeeping; stop
only when the existing drop rule or a valid upper bound excludes improvement
beyond the boundary. A conservative bound might save much less work than this
heuristic, so speed must be measured rather than assumed. Independent 64-base
alignments appended from the last best endpoint would also change affine-gap
costs and path choices and are not equivalent to continued DP.

Raw samples, output/coverage metrics and attempt counts:
[adaptive-extension-results-2026-10-23.json](adaptive-extension-results-2026-10-23.json).
Prototype source, driver, test and PAFs remain under
/private/tmp/faqt-adaptive-extension/. The experiment flag and global diagnostic
counters exist only in this temporary copy and must not be copied into production.
