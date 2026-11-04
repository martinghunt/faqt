# Minimap2 attribution

The assembly chaining transition in `align/chain.go` adapts the coverage reward
and linear-plus-logarithmic gap penalty in minimap2 v2.30 `lchain.c`, principally
`comput_sc` and `comput_sc_simple`:

https://github.com/lh3/minimap2/blob/v2.30/lchain.c

The existing canonical minimizer sketch in `minimizer/sketch.go` follows the
non-homopolymer-compressed sketching approach in minimap2 `sketch.c`:

https://github.com/lh3/minimap2/blob/v2.30/sketch.c

Short-indel normalization in `align/normalize.go` adapts the left-alignment idea
from minimap2 v2.31 `mm_fix_cigar`, restricted to verified exact runs and shifts
shorter than the seed length to protect repeat placements:

https://github.com/lh3/minimap2/blob/v2.31/align.c

The upstream MIT copyright and permission notice is included here. Retain this
notice when distributing the derived code. Faqt's assembly engine is not an
output-compatible port of minimap2: exact-run compaction, repeat-preserving
backtracking, bidirectional union, deduplication, and the scalar Go DP differ.
There is no runtime dependency on minimap2, ksw2, a C compiler, or SIMD libraries.
