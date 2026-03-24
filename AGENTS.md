# faqt agent instructions

## Project purpose

`faqt` is a Go library and CLI for reading, writing, and manipulating biological sequence data.

This project is intentionally **sequence-only**.

Supported sequence inputs:

* FASTA
* FASTQ
* Clustal
* PHYLIP
* SAM (sequence only)
* BAM (sequence only)
* GenBank (sequence only)
* EMBL (sequence only)
* GFF3 (sequence only, from `##FASTA` section only)

All inputs normalize to a minimal sequence record.

---

## Core data model

Keep the core record minimal:

```go
type SeqRecord struct {
    Name        string
    Description string
    Seq         []byte
    Qual        []byte // nil unless FASTQ
}
```

Rules:

* Do NOT add annotation types
* Do NOT add feature models
* Do NOT add metadata maps

---

## Non-negotiable design rules

### 1. Sequence-only

* No annotation graph
* No feature model
* No sequence reconstruction from annotations

### 2. Streaming first

* Readers must stream records
* Do not read entire files into memory unless unavoidable

### 3. Content-based detection

* Detect file format from contents, NOT filename
* Detect compression from magic bytes, NOT filename

### 4. Minimal allocations

* Prefer `[]byte`
* Avoid unnecessary conversions
* Keep FASTQ parsing efficient

### 5. Thin CLI

* CLI uses Cobra
* CLI only parses flags and calls library code
* No parsing logic in `cmd/faqt`

---

## Package responsibilities

* `seqio`: public API, SeqRecord, Reader/Writer, open helpers
* `fasta`, `fastq`, `clustal`, `phylip`, `sam`, `bam`, `genbank`, `embl`, `gff3`: format parsers
* `seq`: sequence utilities
* `orf`: ORF finding
* `internal/sniff`: format detection
* `internal/xopen`: compression handling
* `cmd/faqt`: CLI only

Do not mix responsibilities.

---

## Format-specific rules

### FASTA

* Multi-record required
* Wrapped/unwrapped supported
* Configurable wrap width

### FASTQ

* Multi-record required
* Strict validation of seq/qual length

### Clustal

* Read aligned sequences from multi-block input
* Preserve gaps as sequence characters
* Multi-record required

### PHYLIP

* Read relaxed sequential PHYLIP
* Preserve aligned sequence content
* Multi-record required

### SAM

* Read sequence and quality from alignment records only
* Ignore alignment data except reverse-strand orientation
* Reverse sequence and quality back to original read orientation when reverse-strand flag is set
* Multi-record required

### BAM

* Read sequence and quality from BAM alignment records only
* Ignore alignment data except reverse-strand orientation
* Reverse sequence and quality back to original read orientation when reverse-strand flag is set
* Multi-record required

### GenBank

* Read sequence from `ORIGIN`
* Multi-record required
* Ignore features

### EMBL

* Read sequence from `SQ`
* Multi-record required
* Ignore features

### GFF3

* Only read sequence from `##FASTA`
* Multi-record required
* If no sequence present → return error
* Do not parse features

---

## Output model

Three layers:

1. `String()` → debug only

   * FASTQ if `Qual != nil`
   * FASTA otherwise

2. `WriteTo(io.Writer)` → efficient output

3. `seqio.Writer` → full control (used by CLI)

Always prefer `seqio.Writer` in real usage.

---

## CLI behavior

* Default input: stdin
* Default output: stdout
* Support `-` explicitly
* Detect format by contents
* Detect compression by contents
* Output compression:

  * by extension OR `--compress`
  * flag overrides extension
* stdout defaults to uncompressed

---

## Testing expectations

* Use table-driven tests
* Cover multi-record input
* Cover compression detection
* Cover stdin/stdout
* Cover misleading filenames
* Cover all formats

## Test discipline

Tests are required for all non-trivial changes.

Rules:

* Every new feature must include tests in the same change
* Every new input format must have direct tests in its own package
* Every new CLI command must have at least one command-level test
* Bug fixes should include a regression test whenever practical
* Do not consider work complete until `go test ./...` passes
* If a change is hard to test, explain why explicitly

---

## Non-goals

Do NOT implement:

* annotation models
* feature parsing
* GFF3 feature graph
* sequence reconstruction
* plugin systems
* heavy concurrency pipelines

---

## Change discipline

Before adding code:

* Adds annotation? → reject
* Uses filename detection? → reject
* Breaks streaming? → redesign
* Adds logic to CLI? → move to library

---

## Summary

Keep faqt:

* small
* fast
* predictable
* sequence-focused
