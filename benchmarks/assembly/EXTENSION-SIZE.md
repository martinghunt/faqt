# Contribution of outward end extension — 2026-10-23

Apple M4 Pro; Go 1.27.2; SIMD; GOMAXPROCS=1; one exploratory fresh-process run per setting. K=15, W=10, full two-piece affine DP, unchanged filters and scoring, MaxAnchors=20M. Only the maximum outward extension length per end changes. Internal gaps between exact anchors remain aligned at every setting, including zero. Production defaults are unchanged.

| Maximum bases per end | Original comparison seconds | Original paired positions retained | Repeats comparison seconds | Repeats paired positions retained |
| ---: | ---: | ---: | ---: | ---: |
| 0 | 0.673 | 88.41704% | 3.947 | 99.44800% |
| 16 | 0.881 | 91.83049% | 4.857 | 99.72158% |
| 32 | 1.114 | 95.89509% | 5.841 | 99.82158% |
| 64 | 1.641 | 98.88281% | 8.075 | 99.90594% |
| 128 | 2.921 | 99.77859% | 13.512 | 99.94544% |
| 256 | 5.504 | 100.00000% | 24.536 | 100.00000% |

Retention is agreement with the full 256-base reference, keyed by both contigs, strand, and aligned coordinates. It includes alternative placements and path differences. It is not the fraction of the genome covered or a ground-truth accuracy estimate. Timings exclude input preparation and PAF output.

## What full extension adds versus zero extension

| Dataset | Reference pairs missing without extension | Newly covered A bases | Newly covered B bases |
| --- | ---: | ---: | ---: |
| debug | 357,776 | 0 | 26 |
| repeats | 200,592 | 2,250 | 1,183 |

A is 31585_6#49.fasta / repeats.CP026802.1.fa; B is barcode10_ICESp23_spliced.fasta / repeats.CP151312.1.fa. Newly covered bases count each genomic position once, regardless of repeat partners. Full extension contributes 11.58296% of baseline paired positions on the original pair and 0.55200% on repeats. It covers very little additional unique sequence on these fixtures; most differences concern extra pair placements and alignment ends.

These are final-output comparisons. End extension also changes which candidates pass minimum length, identity and score filters and which paths deduplicate. The differences therefore are not simply a sum of appended tail lengths, and shorter settings can have some coordinate pairs absent from the full-extension result. Match counts need not vary monotonically with the limit.

## A shorter cap

A 64-base cap runs 3.35× faster on the original pair and 3.04× faster on repeats in this exploratory sweep, retaining 98.88281% and 99.90594% of baseline paired positions. Original-pair one-dimensional coverage is unchanged at that cap; repeat coverage loses 1,083 A bases and 371 B bases. A 128-base cap retains 99.77859% and 99.94544%, with smaller speed gains. Shorter caps are sensitivity/performance choices, not equivalent-output optimizations.

All six original-pair settings recover the motivating missing-region match under the original regression interval/identity condition. Every PAF passed independent DNA replay; full-limit output hashes match prior validated results. No production setting or library source was changed.

Raw measurements and coverage/replay metrics: [extension-size-results-2026-10-23.json](extension-size-results-2026-10-23.json). The benchmark accepts FAQT_BENCH_EXTENSION in the temporary copy; it is not a production environment option. The normal library option is align.Options.Extension.
