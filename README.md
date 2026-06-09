# faqt

`faqt` is a command line interface and Go library for reading, writing, and manipulating biological sequence data. It is intentionally sequence-only: every supported input format normalizes into a minimal record with no annotation model, no feature graph, and no sequence reconstruction from annotations. It is a Go reimplementation of the python [Fastaq](https://github.com/sanger-pathogens/Fastaq). This repository was developed with substantial coding assistance from [OpenAI Codex](https://openai.com/codex), which helped with implementation, refactoring, tests, documentation, and benchmarking under human direction and review.

## Install

The simplest way to install `faqt` is to download the latest prebuilt binary from the GitHub releases page:

- https://github.com/martinghunt/faqt/releases/latest

Choose the archive or binary matching your OS and CPU architecture.
Release pages also include a SHA-256 checksum file named like `faqt-v0.2.1-checksums.txt`.
Use it to verify downloaded archives before installing.

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
- `faqt interleave`: interleave two sequence files
- `faqt to-perfect-reads`: simulate perfect FASTQ reads from a reference
- `faqt make-random-contigs`: make random FASTA contigs
- `faqt stats`: report assembly-style sequence statistics (reimplementation of [assembly-stats](https://github.com/sanger-pathogens/assembly-stats))
- `faqt download`: download genome or sequence data by accession

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
faqt interleave reads_1.fq reads_2.fq -o reads_interleaved.fq --suffix1 /1 --suffix2 /2
faqt to-perfect-reads ref.fa --out reads.fq --coverage 50 --read-length 150
faqt to-perfect-reads ref.fa --forward-out reads_1.fq --reverse-out reads_2.fq --mean-insert 300 --insert-std 30 --coverage 50 --read-length 150
faqt make-random-contigs 10 500 -o contigs.fa --seed 1
faqt download GCF_000001405.40 -o genome.gff3
faqt download GCF_000001405.40 -o genome.gff3.gz
faqt download GCF_000001405.40 -o genome.fa --fasta
faqt download WP_002248791.1 -o protein.fa
faqt download WP_002248791.1 --nucleotide -o cds.fa
faqt download WP_002248791.1 --nucleotide=all --source all -o all-cds.fa
faqt download WP_002248791.1 --nucleotide --assembly GCF_000191525.1 -o cds.fa
cat reads.gb | faqt to-fasta
faqt to-fasta -i - -o out.fa < reads.embl
faqt stats assembly.fa
faqt stats -t assembly.fa
```

`download` routes `GCA_` and `GCF_` accessions to genome download, and other accessions to sequence FASTA download. Genome downloads treat compression suffixes separately from biological format suffixes. For example, `.gz`, `.bz2`, `.xz`, and `.zst` choose output compression. If the selected downloaded genome content conflicts with a recognized biological suffix such as `.fa`, `.gff3`, `.gbff`, or `.embl`, the command writes the requested path and prints a non-fatal warning.

For sequence accessions, `download` writes FASTA only. Its `--db` flag accepts `auto`, `protein`, `nuccore`, `nucleotide`, or `sequences`; `auto` routes common protein accessions such as `WP_002248791.1` to NCBI Protein, so `faqt download WP_002248791.1` downloads the protein sequence by default. Use `--nucleotide` to download the first RefSeq CDS nucleotide sequence linked from a protein accession, or `--nucleotide=all` to write all matching CDS records. `--source` accepts `refseq`, `insdc`, or `all` and defaults to `refseq`; `--assembly` filters nucleotide CDS rows to one assembly accession. Set `NCBI_API_KEY` and `NCBI_EMAIL`, or pass `--api-key` and `--email`, when you want those values sent with sequence requests.

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

For small inputs where an in-memory representation is useful, `seqio.ReadAllPath` returns records in file order and `seqio.ReadAllByNamePath` returns records keyed by name:

```go
records, err := seqio.ReadAllByNamePath("refs.fa.gz")
if err != nil {
	log.Fatal(err)
}
fmt.Println(len(records["ref1"].Seq))
```

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

### Interleaving Streams

Use `seqio.Interleave` when you already have readers and a writer. It writes alternating records from the first and second inputs, and returns an error if either input has an unmatched record.

```go
err := seqio.Interleave(
	reader1,
	reader2,
	writer,
	seqio.InterleaveOptions{Suffix1: "/1", Suffix2: "/2"},
)
if err != nil {
	log.Fatal(err)
}
```

For path-based use, `seqio.InterleavePath` detects input formats and compression from contents, infers FASTA or FASTQ output from the first pair, and applies normal output compression options.

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
- `seq.NormalizeDNA`
- `seq.TranslateCodon`
- `seq.Translate`

The `orf` package provides ORF finding:

- `orf.FindORFs`

### Statistics and Generated Data

The `stats` package computes assembly-style sequence statistics from any supported input format:

```go
s, err := stats.FromPath("assembly.fa.gz", 0)
if err != nil {
	log.Fatal(err)
}
fmt.Print(s.String(stats.FormatHuman))
```

Use `stats.RenderMany` to render multiple `stats.Stats` values in a shared output format. The available formats are `stats.FormatHuman`, `stats.FormatTab`, `stats.FormatTabNoHeader`, and `stats.FormatGreppy`.

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

Perfect FASTQ reads can be simulated from a streaming reference reader with the `perfectreads` package:

```go
reader, err := seqio.OpenPath("ref.fa")
if err != nil {
	log.Fatal(err)
}
if closer, ok := reader.(io.Closer); ok {
	defer closer.Close()
}
writer, err := seqio.CreateFASTQPath("reads.fq.gz")
if err != nil {
	log.Fatal(err)
}
defer writer.Close()

report, err := perfectreads.GenerateSingle(reader, writer, perfectreads.Options{
	Coverage:   50,
	ReadLength: 150,
	Seed:       1,
})
if err != nil {
	log.Fatal(err)
}
_ = report
```

The `genomedl` package exposes `genomedl.DownloadGenome(accession, outPath)` for downloading one genome accession. By default it writes an available annotation file (GFF3 with embedded FASTA, GenBank/GBFF, or EMBL), and falls back to FASTA when no annotation file is available. Use `genomedl.DownloadGenomeWithOptions` with `genomedl.DownloadOptions{FastaOnly: true}` to force FASTA output.

The `seqdl` package exposes `seqdl.DownloadAccession(accession, outPath, options)` and `seqdl.DownloadAccessions(accessions, outPath, options)` for downloading accession FASTA from NCBI EFetch. Downloaded content is streamed through `seqio` and written as FASTA.

### Minimizers, Mapping, and Alignment

The `minimizer` package builds minimizer indexes and sketches query sequences:

```go
index, err := minimizer.BuildFromPath("ref.fa", minimizer.Options{
	K: 15,
	W: 10,
})
if err != nil {
	log.Fatal(err)
}

anchors := index.Query([]byte("ACGTTGCA"))
_ = anchors
```

For the common mapping workflow, use the higher-level `mapping` package. It builds a minimizer index, finds candidate hits, and runs the default aligner unless you set `Mapper.Aligner` to `nil`.

```go
m, err := mapping.BuildFromPath("ref.fa", minimizer.Options{
	K:         15,
	W:         10,
	MidOcc:    100,
	MaxMaxOcc: 500,
	OccDist:   500,
	QOccFrac:  0.01,
})
if err != nil {
	log.Fatal(err)
}

result, err := m.Map("query1", []byte("ACGTTGCA"))
if err != nil {
	log.Fatal(err)
}
for _, hit := range result.Hits {
	fmt.Println(hit.RefName, hit.Alignment.Score, hit.Alignment.CIGAR)
}
```

The lower-level `mapper` and `align` packages are available when you need to customize the pipeline:

- `mapper.DefaultPipeline`, `mapper.Map`, and `mapper.ExtractCandidates` expose anchor clustering, chaining, and candidate extraction.
- `align.DefaultAligner` and `align.AlignCandidates` expose candidate alignment and ranking.

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
