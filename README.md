# faqt

`faqt` is a command line interface and Go library for reading, writing, and manipulating biological sequence data. It is intentionally sequence-only: every supported input format normalizes into a minimal record with no annotation model, no feature graph, and no sequence reconstruction from annotations. It is a Go reimplementation of the python [Fastaq](https://github.com/sanger-pathogens/Fastaq).

## Install

The simplest way to install `faqt` is to download the latest prebuilt binary from the GitHub releases page:

- https://github.com/martinghunt/faqt/releases/latest

Choose the archive or binary matching your OS and CPU architecture.

After installing, check the version with:

```bash
faqt --version
```

If you want to build locally instead:

```bash
./build.sh
```

That builds `faqt` for the current OS and architecture into `./build/faqt` or `./build/faqt.exe`.
Local builds report version `dev` unless you pass an explicit release version.

## Command Line Usage

`faqt` currently provides these command-line tasks:

- `faqt to-fasta`: convert supported input formats to FASTA
- `faqt to-perfect-reads`: simulate perfect FASTQ reads from a reference
- `faqt stats`: report assembly-style sequence statistics (reimplementation of [assembly-stats](https://github.com/sanger-pathogens/assembly-stats))
- `faqt download-genome`: download a genome and save it to one output file

Command-line input can be any supported sequence format:

- FASTA
- FASTQ
- GenBank
- EMBL
- GFF3 sequence from `##FASTA`
- SAM
- BAM
- Clustal
- PHYLIP


Compressed input files are handled automatically by content, not filename. `faqt` can read:

- uncompressed files
- gzip
- bzip2
- xz
- zstd

For file output, compression is chosen by output filename suffix or by `--compress` when a command supports it:

- `.gz` gives gzip output
- `.bz2` gives bzip2 output
- `.xz` gives xz output
- `.zst` gives zstd output

When writing to `stdout`, output is uncompressed unless compression is explicitly requested.

For command-line I/O, `-` means standard streams:

- use `-` as the input path to read from `stdin`
- use `-` as the output path to write to `stdout`
- `to-fasta` defaults to `stdin`/`stdout` when paths are not given

Examples:

```bash
faqt to-fasta reads.fq.gz > out.fa
faqt to-fasta --input aln.aln --remove-dashes
faqt to-perfect-reads ref.fa --out reads.fq --coverage 50 --read-length 150
faqt to-perfect-reads ref.fa --forward-out reads_1.fq --reverse-out reads_2.fq --mean-insert 300 --insert-std 30 --coverage 50 --read-length 150
faqt download-genome GCF_000001405.40 -o genome.gff3
cat reads.gb | faqt to-fasta
faqt to-fasta -i - -o out.fa < reads.embl
faqt stats assembly.fa
faqt stats -t assembly.fa
```

## Philosophy

- Sequence-only core model.
- Streaming readers for all supported formats.
- Content-based format detection after decompression.
- Thin Cobra CLI that delegates all real work to library packages.
- Minimal allocations with `[]byte`-centric APIs.

## Minimal Record Type

```go
type SeqRecord struct {
    Name        string
    Description string
    Seq         []byte
    Qual        []byte // nil unless FASTQ
}
```

## Repository Layout

```text
/cmd/faqt        Cobra CLI
/seqio              public API and open helpers
/fasta              FASTA reader/writer
/fastq              FASTQ reader/writer
/clustal            Clustal alignment reader
/phylip             relaxed PHYLIP reader
/genbank            GenBank sequence reader
/embl               EMBL sequence reader
/gff3               GFF3 sequence reader from ##FASTA only
/seq                sequence transforms and gap utilities
/orf                ORF finding
/internal/sniff     content-based format detection
/internal/xopen     compression-aware open/create helpers
```

## Supported Input Formats

- FASTA, multi-record
- FASTQ, multi-record with strict sequence/quality length validation
- Clustal, multi-block aligned sequences with dashes preserved
- PHYLIP, relaxed sequential multi-record
- SAM, multi-record, sequence from the sequence column with reverse-strand reads converted back to original orientation
- BAM, multi-record, sequence from the BAM sequence field with reverse-strand reads converted back to original orientation
- GenBank, multi-record, sequence from `ORIGIN`
- EMBL, multi-record, sequence from `SQ`
- GFF3, multi-record, sequence from `##FASTA` only

For SAM/BAM input, alignment data are ignored apart from the reverse-strand flag, which causes sequence and quality to be reversed into original read orientation. GFF3 inputs without a `##FASTA` section return an error. Features are ignored everywhere.

## Compression Support

Input compression is detected by magic bytes, never filenames:

- uncompressed
- gzip
- bzip2
- xz
- zstd

Output compression is selected by path suffix or `WithCompression` / `--compress`. Supported suffixes:

- `.gz` -> gzip
- `.bz2` -> bzip2
- `.xz` -> xz
- `.zst` -> zstd

When writing to stdout, output is uncompressed unless compression is explicitly requested.

## Format Detection

Biological format detection happens after decompression and is based on buffered content peeks:

- FASTA: first non-space byte is `>`
- FASTQ: leading `@` with FASTQ-like first record structure
- Clustal: leading `CLUSTAL` header
- PHYLIP: first line contains sequence count and alignment length
- SAM: header lines like `@HD`/`@SQ` or SAM alignment line structure
- BAM: BGZF-compressed BAM content detected before generic text decompression
- GenBank: leading `LOCUS`
- EMBL: leading `ID`
- GFF3: leading `##gff-version 3`

## Public API

```go
reader, err := seqio.OpenPath("reads.fq.gz")
if err != nil {
    panic(err)
}

for {
    rec, err := reader.Read()
    if err == io.EOF {
        break
    }
    if err != nil {
        panic(err)
    }
    fmt.Println(rec.Name, len(rec.Seq))
}
```

```go
writer, err := seqio.CreatePath("out.fa.gz", seqio.FASTA, seqio.WithWrap(60))
if err != nil {
    panic(err)
}
defer writer.Close()

_ = writer.Write(&seqio.SeqRecord{Name: "r1", Seq: []byte("ACGT")})
```

For the common FASTA/FASTQ cases, use the format-specific constructors and keep the wrapping choice on the writer:

```go
writer, err := seqio.CreateFASTAPath("out.fa.gz", seqio.WithWrap(60))
if err != nil {
    panic(err)
}
defer writer.Close()

for _, rec := range records {
    _ = writer.Write(&rec)
}
```

`OpenPath("-")` reads stdin. `CreatePath("-")` writes stdout.

For library-level file operations, `seqio` also exposes reusable streaming helpers:

```go
err := seqio.ToFASTAPath("reads.fq.gz", "reads.fa", seqio.WithWrap(60))
```

```go
err := seqio.TransformPath("reads.fa", "reads.rc.fa", seqio.FASTA, func(rec *seqio.SeqRecord) (*seqio.SeqRecord, error) {
    copyRec := *rec
    copyRec.Seq = seq.ReverseComplement(rec.Seq)
    return &copyRec, nil
})
```

## Printing Model

There are three output layers:

1. `String()` for convenience and debugging.
2. `WriteTo(io.Writer)` for efficient low-level output.
3. `seqio.Writer` for canonical configurable output with format, wrapping, and compression control.

`String()` emits FASTQ if `Qual != nil`, otherwise FASTA. It does not wrap sequence lines.

## Sequence Utilities

`seq` provides:

- `ReverseComplement`
- `Subseq`
- `FindGaps`

`orf` provides:

- `FindORFs` with configurable start codons, minimum length, and forward or both-strand scanning

## CLI

The repository includes a Cobra CLI binary named `faqt`.

```bash
faqt to-fasta reads.fq.gz > out.fa
cat reads.gb | faqt to-fasta
faqt to-fasta --input reads.gff3.zst --output out.fa.gz --wrap 60
faqt to-fasta --input aln.aln --remove-dashes
faqt to-perfect-reads ref.fa --out reads.fq --coverage 50 --read-length 150
faqt to-perfect-reads ref.fa --forward-out reads_1.fq --reverse-out reads_2.fq --mean-insert 300 --insert-std 30 --coverage 50 --read-length 150
faqt download-genome GCF_000001405.40 -o genome.gff3
faqt stats assembly.fa
faqt stats -t assembly.fa
faqt stats -s assembly.fa
```

`to-fasta` defaults to stdin/stdout, always emits FASTA, and stays thin by calling the library-level `seqio.ToFASTAPath`.

`to-perfect-reads` simulates perfect FASTQ reads from reference sequences. Single-end mode writes one output file with `--out`. Paired-end mode uses innie orientation and insert sizes sampled from a normal distribution, writing separate forward and reverse output files instead of interleaving. Unlike `fastaq`, `faqt` also uses a slightly more realistic quality tail instead of constant `I` scores.

`stats` reports assembly-style length statistics with the same human, greppy, tab-delimited, and tab-delimited-no-header layouts used by `assembly-stats`. It supports:

- `-l` minimum sequence length cutoff
- `-s` grep-friendly output
- `-t` tab-delimited output
- `-u` tab-delimited output with no header

`download-genome` downloads a genome by accession and writes one output file. If the source provides separate FASTA and GFF3 files, `faqt` combines them into a standard GFF3 file with a `##FASTA` section. If the source provides a single file such as GenBank or EMBL, that original file type is preserved. This is the one deliberate exception to the sequence-only normalisation rule, because downloaded files may include annotation and are saved without parsing it.

## Development

```bash
go test ./...
```

## Building

For a normal local build, run:

```bash
./build.sh
```

For a cross-platform release build:

```bash
./build.sh --release --version v1.2.3
```

That produces binaries for:

- `darwin/amd64`
- `darwin/arm64`
- `linux/amd64`
- `linux/arm64`
- `windows/amd64`
- `windows/arm64`

Release artifact names include the version, for example:

- `faqt-v1.2.3-darwin-arm64`
- `faqt-v1.2.3-linux-amd64`
- `faqt-v1.2.3-windows-amd64.exe`

You can also build a specific target without using release mode:

```bash
./build.sh --os linux --arch arm64
```

The test suite covers multi-record parsing, compression-aware I/O, GFF3 error handling, stdin/stdout behavior, sequence utilities, ORF detection, and CLI conversion.
