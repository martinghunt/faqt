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
- `faqt make-random-contigs`: make random FASTA contigs
- `faqt stats`: report assembly-style sequence statistics (reimplementation of [assembly-stats](https://github.com/sanger-pathogens/assembly-stats))
- `faqt download-genome`: download a genome and save it to one output file

Use `faqt -h` or `faqt --help` for top-level help. Use `-h` or `--help` after a command name to see command-specific flags and examples, for example:

```bash
faqt to-fasta --help
faqt stats -h
```

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
faqt make-random-contigs 10 500 -o contigs.fa --seed 1
faqt download-genome GCF_000001405.40 -o genome.gff3
cat reads.gb | faqt to-fasta
faqt to-fasta -i - -o out.fa < reads.embl
faqt stats assembly.fa
faqt stats -t assembly.fa
```

## Public API

Use `faqt` as a Go library by importing the package that matches the level of control you need. Most applications should start with `seqio`, which provides the public sequence record type, streaming readers and writers, compression-aware path helpers, and format detection.

```go
import "github.com/martinghunt/faqt/seqio"
```

The core record is intentionally minimal:

```go
type SeqRecord struct {
    Name        string
    Description string
    Seq         []byte
    Qual        []byte // nil unless FASTQ
}
```

All supported inputs normalize to this record. `Qual` is `nil` unless the source has FASTQ-style qualities. The library does not expose annotation models, feature graphs, or metadata maps.

### Reading Records

`seqio.OpenPath` opens a path, detects compression from magic bytes, detects the biological format from content, and returns a streaming reader. Use `"-"` to read from standard input.

```go
package main

import (
	"fmt"
	"io"
	"log"

	"github.com/martinghunt/faqt/seqio"
)

func main() {
	reader, err := seqio.OpenPath("reads.fq.gz")
	if err != nil {
		log.Fatal(err)
	}
	if closer, ok := reader.(io.Closer); ok {
		defer closer.Close()
	}

	for {
		rec, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s\t%d\n", rec.Name, len(rec.Seq))
	}
}
```

Use `seqio.OpenReader` when you already have an `io.Reader`.

Supported input formats are FASTA, FASTQ, Clustal, PHYLIP, SAM, BAM, GenBank, EMBL, and GFF3 sequence from the `##FASTA` section. For SAM and BAM, alignment data are ignored except for the reverse-strand flag, which causes sequence and quality to be reversed back to original read orientation. GFF3 inputs without a `##FASTA` section return an error.

### Writing Records

`seqio.CreatePath` creates a streaming writer for FASTA or FASTQ. Compression is selected from the output suffix by default, or explicitly with `seqio.WithCompression`. Use `"-"` to write to standard output; standard output is uncompressed unless compression is explicitly requested.

```go
writer, err := seqio.CreatePath("out.fa.gz", seqio.FASTA, seqio.WithWrap(60))
if err != nil {
	log.Fatal(err)
}
defer writer.Close()

err = writer.Write(&seqio.SeqRecord{
	Name: "read1",
	Seq:  []byte("ACGT"),
})
if err != nil {
	log.Fatal(err)
}
```

For common cases, the format-specific helpers make the output format explicit:

```go
writer, err := seqio.CreateFASTAPath("out.fa.gz", seqio.WithWrap(60))
if err != nil {
	log.Fatal(err)
}
defer writer.Close()
```

Available writer formats are `seqio.FASTA` and `seqio.FASTQ`. FASTA writers support `seqio.WithWrap(width)`. Output compression can be forced with `seqio.WithCompression(seqio.CompressGzip)`, `seqio.CompressBzip2`, `seqio.CompressXZ`, `seqio.CompressZstd`, or disabled with `seqio.CompressNone`.

### Transforming Streams

Use `seqio.Process` when you already have a reader and writer, or `seqio.TransformPath` for path-based streaming transforms.

```go
err := seqio.TransformPath(
	"reads.fa",
	"reads.rc.fa",
	seqio.FASTA,
	func(rec *seqio.SeqRecord) (*seqio.SeqRecord, error) {
		out := *rec
		out.Seq = seq.ReverseComplement(rec.Seq)
		return &out, nil
	},
	seqio.WithWrap(60),
)
if err != nil {
	log.Fatal(err)
}
```

For straight conversion to FASTA:

```go
err := seqio.ToFASTAPath("reads.fq.gz", "reads.fa", seqio.WithWrap(60))
if err != nil {
	log.Fatal(err)
}
```

Returning `nil` from a transform skips that record.

### Lower-Level Format Packages

The format packages expose direct readers and writers where available:

- `fasta` and `fastq`: format-specific readers and writers
- `clustal`, `phylip`, `sam`, `bam`, `genbank`, `embl`, `gff3`: format-specific readers

Use these when the input format is already known and you do not need `seqio` format or compression detection.

### Sequence Utilities

The `seq` package provides byte-slice sequence helpers:

- `seq.ReverseComplement`
- `seq.Subseq`
- `seq.FindGaps`

The `orf` package provides ORF finding:

- `orf.FindORFs`

Random FASTA contigs can be generated through the `randomcontigs` package:

```go
seed := int64(1)
err := randomcontigs.GenerateToPath("contigs.fa", randomcontigs.Options{
	Contigs:     10,
	Length:      500,
	Seed:        &seed,
	FirstNumber: 1,
})
if err != nil {
	log.Fatal(err)
}
```

### Output Layers

There are three output layers:

1. `SeqRecord.String()` for convenience and debugging.
2. `SeqRecord.WriteTo(io.Writer)` for efficient low-level output.
3. `seqio.Writer` for configurable output with format, wrapping, and compression control.

Prefer `seqio.Writer` for normal library use. `String()` emits FASTQ if `Qual != nil`, otherwise FASTA, and does not wrap sequence lines.

### Supported Formats and Compression

Input format detection is content-based after decompression. Compression detection uses magic bytes, not filenames. Supported input compression:

- uncompressed
- gzip
- bzip2
- xz
- zstd

Output compression is selected by path suffix or `seqio.WithCompression`:

- `.gz` for gzip
- `.bz2` for bzip2
- `.xz` for xz
- `.zst` for zstd

## Development and Building

Run the test suite before considering changes complete:

```bash
go test ./...
```

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

The test suite covers multi-record parsing, compression-aware I/O, GFF3 error handling, stdin/stdout behavior, sequence utilities, random contig generation, ORF detection, and CLI conversion.
