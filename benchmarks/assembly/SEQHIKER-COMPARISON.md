# Seqhiker current engine versus faqt SIMD — 2026-10-23

Apple M4 Pro; Go 1.27.2; GOMAXPROCS=1; three fresh-process runs per engine and dataset. Runs alternated old/new, serially. Both use the unchanged original FASTAs. Seqhiker source was copied from the current working tree, including its pre-existing uncommitted comparison changes; no application source was edited. Faqt uses the current shared NEON SIMD backend, full end extension and a raised 20M anchor budget so the repeat-rich comparison completes.

## Initial comparison latency

Median comparison seconds exclude input loading and output serialization. The old engine is Seqhiker’s real initial-block API, not the earlier scalar faqt implementation.

| Dataset | Existing Seqhiker | New faqt SIMD | New / old | Old summaries | New paths |
| --- | ---: | ---: | ---: | ---: | ---: |
| Original debug pair | 2.245 s | 5.537 s | 2.47× | 2,720 | 7,116 |
| Repeat-rich pair | 2.987 s | 24.646 s | 8.25× | 1,024 | 42,414 |

The current Seqhiker engine is faster for initial comparison on both fixtures. This comparison does not establish a speed improvement over the existing application engine.

## Work and output differences

Seqhiker invokes NewEngine, AddComparisonGenome for each file, then GetComparisonBlocksByGenomes. Its initial output contains block summaries; detailed CIGARs/variants are generated on demand. Faqt ReadGenome/Compare returns full alignment paths immediately. Neither application UI rendering nor old on-demand detail retrieval is timed.

Seqhiker limits seed occurrences to 64, has a per-segment-pair refinement limit of 1,024 accepted blocks, skips candidates beyond an 80M-cell refinement budget, and uses different chaining, filtering and redundancy rules. The repeat-rich result is exactly 1,024 blocks. Faqt retains more alternative paths and uses full DP extension, with different explicit resource budgets. Seqhiker's minimum identity is 50%, versus 70% in the new benchmark. Counts are different output units under different policies; they are not interchangeable accuracy or biological-copy measurements.

Every timed faqt PAF exactly matches the previously DNA-replayed reference hash. Every old block JSON is identical across its three repeats. This check does not assert equality between old and new outputs.

## Whole-process measurements

Whole-process seconds include loading, preparation, comparison, test-runner startup and output serialization. Old output serialization writes block JSON; new output writes PAF, so the formats differ. First-process launch overhead is visible in the old original-pair sample; the table uses medians.

| Dataset | Existing Seqhiker wall | New faqt wall |
| --- | ---: | ---: |
| debug | 2.337 s | 5.757 s |
| repeats | 3.195 s | 25.213 s |

Raw samples and input/output hashes: [seqhiker-comparison-results-2026-10-23.json](seqhiker-comparison-results-2026-10-23.json). Build metadata and source hashes are embedded in the same results JSON. The isolated driver, original-engine adapter and private PAF/block outputs were kept locally.
