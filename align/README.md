# Assembly comparison

`align` is a pure Go DNA assembly-comparison engine. It replaces the previous
`align`, `mapper`, and `mapping` read-mapping-style pipeline. This is a breaking
library API change. `minimizer` remains available as an independent low-level
package; its query-side occurrence filters are not used by assembly comparison.

## API

```go
opts := align.DefaultOptions()
opts.MinLength = 100
opts.MinIdentity = 0.90
opts.Progress = func(p align.Progress) {
    fmt.Printf("%d / %d\n", p.Completed, p.Total)
}

// seqio opens FASTA or other supported sequence inputs by content.
readerA, err := seqio.OpenPath("genome-a.fa")
if err != nil { return err }
// Close the opened reader according to the application's normal ownership rules.
a, err := align.ReadGenome(readerA, align.SketchOptions{})
if err != nil { return err }
// Likewise open and prepare genome B. Prepare([]seqio.SeqRecord, ...) is also
// available for sequences that the application already has in memory.
b, err := align.Prepare(recordsB, align.SketchOptions{})
if err != nil { return err }

matches, err := align.Compare(ctx, a, b, opts)
if err != nil { return err }
for _, m := range matches {
    fmt.Println(a.ContigName(m.AContig), m.A,
        b.ContigName(m.BContig), m.B, m.Reverse, m.Identity(), m.CIGAR())
}
```

Prepared genomes own normalized sequence copies and indexes, and can be reused
or compared concurrently. Sequence storage is necessary for random-access
alignment. No annotations, qualities, application state or display concatenation
are included. Contig indices preserve input record order, including duplicate
names. Empty contigs are allowed. Upper/lower case and U/T normalize identically;
ambiguous IUPAC bases normalize to N. N breaks seeds and always scores as a
substitution, even against another N. Invalid DNA bytes return an error.

Both genomes must use the same sketch options. Default seeds have K=15 and W=10;
K must be odd, between 3 and 31. Use `DefaultOptions()` for comparison options,
then change fields; `Options{}` is deliberately invalid.

## Symmetry and repeats

The engine orders genomes by a digest of normalized contig content, searches in
both directions, converts results to the canonical axes, and deduplicates there.
Only then are results mapped back to the caller's axes. Names and loading order
never choose which search wins. Equal-content genomes close their result set
under swapping, retaining both directions of off-diagonal self repeats.

`Compare(a,b)` and `Compare(b,a)` therefore yield the same matches after applying
`Match.Swap()` and `SortMatches`. This includes CIGAR operations, not merely
coverage or endpoints. For reverse matches, swapping reverses operation order
and exchanges I/D. Reordering contigs changes their indices and is a separate
operation from exchanging the two genomes.

Multiple placements are first-class results. No primary/secondary distinction,
exclusive anchor consumption, top-N filter, percentage-overlap suppression or
maximum block count is used. Deduplication removes only identical alignment
subpaths on the same pair of contigs and the same strand. Two paths sharing one
axis, or using different copies of a repeat, remain distinct.

This is still a **seed-based heuristic**, not mathematical enumeration of every
local alignment above an identity threshold. A homologous region must contain
usable seeds and a sufficiently scoring chain. In particular, very short,
highly divergent or ambiguous regions can be missed. Thresholds define qualifying
reported matches; they do not guarantee exhaustive discovery. Repetitive inputs
can have a genuinely quadratic number of distinct placements.

## Algorithm

1. Canonical hashed minimizers on both genomes; retain every seed occurrence.
2. Verify seed matches against DNA, then extend and compact collinear seeds into
   maximal exact runs. Hash collisions cannot create an assumed exact match.
3. Dynamic-programming chaining on exact runs, without fixed diagonal bins.
   Transitions reward covered sequence and penalize diagonal displacement.
   Backtracking retains all best-predecessor leaves and allows shared ancestors;
   significant standalone exact runs are candidates as well.
4. Align chain gaps with banded, two-piece affine-gap DP. Traceback uses
   two bytes per visited cell; scores retain two rolling rows. The initial band
   includes the endpoint length difference plus padding and widens until an outside-band score bound proves that no better
   global path was excluded, or until the full matrix is covered.
5. Split unsupported joins using score drop, and extend chain ends with anchored
   DP. Gap costs are the lower of two affine costs, supporting short and long
   indels. The default maximum unanchored gap is 2,000 bases; larger transitions
   become separate local matches. Maximum unseeded end extension is 256 bases.
6. Union both search directions, apply minimum length, identity and score, and
   remove identical subpaths.

The chaining transition is adapted from minimap2 v2.30, with attribution in
`third_party/minimap2`. This is not a bit-for-bit minimap2 port or a claim of
minimap2-equivalent sensitivity. The main remaining performance risks are
quadratic chaining in dense repeat regions and repeated base alignment. The
normal build uses portable Go. An experimental SIMD kernel can be enabled with
`GOEXPERIMENT=simd` on a supporting Go toolchain (tested with Go 1.27.2): NEON
on ARM64 and runtime-guarded AVX2 on amd64. Other amd64 CPUs use scalar DP. It uses the Suzuki-Kasahara score-difference recurrence
with sixteen 8-bit lanes where scoring bounds permit, and preserves the scalar
kernel's scoring, traceback priorities, band and row-order Z-drop. Completed
rows are checked during the anti-diagonal computation. Scratch buffers are
reused with `sync.Pool`; oversized scratch buffers are not retained, and
returned operations own their storage.
Short/narrow or oversized matrices, unsupported scoring ranges and restrictive cell budgets
use the existing four-lane fallback on ARM64 or scalar DP on amd64. There is no external executable,
cgo or handwritten assembly dependency.

Subpath containment uses an index of paired diagonals and gap segments to select
possible containing paths. Every selected candidate still passes the original
CIGAR-level `pathContains` check. No repeat placement is removed merely because
its intervals overlap another match.

See [the kernel study](../benchmarks/assembly/KERNELS.md) for measurements and
wavefront alignment experiments.

## Optional adaptive end extension

Full end DP remains the default. To select the faster heuristic policy:

```go
opts := align.DefaultOptions()
opts.EndExtension = align.EndExtensionAdaptive
```

This starts with 64 bases per end and doubles the search while an edge path
remains within 24 match rewards of the best prefix (48 points with default
scoring), up to `Options.Extension`. It stops on the existing Z-drop rule.
Internal gaps still receive full alignment. Competing paths are retained by
recomputing each larger window from its anchor, rather than appending chunks.

Adaptive mode can miss later score recovery after a poor stretch; it does not
promise the same output as `EndExtensionDP`. On the two current fixtures it
retains 99.971% / 99.985% of full-mode coordinate pairs at about 1.8× / 2.3× the
speed. These are data-specific observations, not sensitivity guarantees. See
[selection experiments and validation](../benchmarks/assembly/ADAPTIVE-STUDY.md).
The mode works with scalar, NEON and AVX2 dispatch; SIMD still requires the
SIMD-enabled build. Reuse the same prepared genomes when changing this mode.

## Coordinates and operations

Intervals are zero-based, half-open and always refer to original forward contig
coordinates. `Reverse` means the sequences align on opposite strands. Operations
traverse A forward and B forward or backward according to `Reverse`:

| Operation | A consumption | B consumption | Meaning |
|---|---:|---:|---|
| `=` | length | length | Exact A/C/G/T match |
| `X` | length | length | Substitution, including N |
| `I` | length | 0 | Bases only in A |
| `D` | 0 | length | Bases only in B |

`Ops` is run-length encoded. `CIGAR()` formats those operations. There are no
clipping operations. `Identity()` is `Matches / Columns`, including each indel
base in the denominator. `MinChainScore` filters chaining coverage scores; `MinScore` separately filters
final base-alignment scores. Scoring uses positive `Match` and positive penalties;
a gap costs `min(GapOpen+n*GapExtend, LongGapOpen+n*LongGapExtend)`.

## Cancellation and resources

`Compare` accepts a context and a synchronous progress callback. Errors and
cancellation return **no partial matches**. The progress callback runs between
contig searches; cancellation is also checked inside expensive computations.

`MaxAnchors` defaults to 2,000,000 verified seed hits per contig pair and strand,
before compaction. `MaxCells` defaults to 16,000,000 visited cells per DP attempt.
Exceeding either returns a wrapped `*align.LimitError`; inspect it with
`errors.As`. Zero disables that limit. Raising `MaxGap`, extension or sensitivity
may require raising the resource budgets too. These budgets never silently
truncate hits. Output itself is not capped.

## Validation

Tests cover input-swap symmetry, complete Cartesian repeat placements in the
synthetic fixtures, tandem repeat length changes, nested duplications, SNPs,
indels, inversions, contig boundaries, ambiguity, cancellation, resource errors,
and immutable preparation. Every integration-test alignment is replayed against
its source DNA to verify intervals, operations, identity and score. The DP is
also checked against an independent exhaustive small-matrix scoring oracle.

See [validation observations](VALIDATION.md) for the original Seqhiker debug pair
and reproducible checks.

See the [indexed-containment and sixteen-lane follow-up](../benchmarks/assembly/OPTIMIZATION.md)
for component benchmarks, unchanged output hashes and current comparisons with minimap2.

The amd64 backend passed native Ryzen 5 7600X validation, including exact
whole-input output hashes and feature-mask fallback tests. See the
[native AVX2 results](../benchmarks/remote/RESULTS-2026-10-23.md) and
[Linux validation bundle](../benchmarks/remote/README.md).
