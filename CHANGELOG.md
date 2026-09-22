# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed
- Read bgzipped text input instead of failing with `sam: magic number mismatch`. BGZF carries both BAM and bgzipped FASTA, FASTQ, SAM, PHYLIP, Clustal, GenBank, EMBL and GFF3, so `seqio` now decompresses the first block and looks for the BAM magic number before choosing a reader. Every text format was unreadable when compressed with `bgzip`, the tool the samtools ecosystem uses.

### Changed
- Reading and writing helpers in `seqio` and `stats` take `seqio.Option` values, which is source compatible for calls but not for code that takes these functions as values: `seqio.OpenPath`, `OpenPathAllowEmpty`, `OpenReader`, `ReadAllPath`, `ReadAllByNamePath`, `CountRecordsPath`, `stats.FromPath` and `stats.FromPaths`. Building faqt as a library now needs a Go 1.25 toolchain.
- Keep faqt on one core unless asked for more. Reading zstd, writing zstd and reading BAM previously ran one worker per core, because the compression libraries default to `GOMAXPROCS`; reading a BAM used 343% CPU, and schedulers that allocate one core kill jobs that fan out. Compression concurrency is now one by default, and `--threads` raises it for the commands that read or write sequence data. Serial decoding also uses less total CPU: the same BAM read costs 0.57 CPU-seconds serially against 0.99 across a pool.
- Buffer output written by `seqio.CreatePath` and `seqio.OpenWriter`, which were writing straight to the file descriptor: one FASTQ record cost five `write` syscalls and a wrapped FASTA line cost two. `faqt to-fasta --wrap 60` on a 231 MB input drops from 10.1s to 0.22s. `Close` now flushes, so its error must be propagated for output to be complete. `seqio.NewWriter` stays unbuffered, because the caller owns the writer and may never call `Close`.
- Read FASTA about 2.9x faster and FASTQ about 1.3x faster by parsing lines out of the read buffer rather than allocating one byte slice per line. Reading a 203 MB wrapped FASTA allocated 1262 MB in 3.34M allocations and now allocates 207 MB in under a thousand; the garbage that produced was also pushing CPU use above one core, so the same read drops from 146% CPU to 93%.
- Decompress gzip with `klauspost/compress`, already a dependency, which is about 21% faster than `compress/gzip` on one core.
- Raise read and write buffers from 4 KB and 8 KB to 256 KB, cutting read syscalls by roughly 64x on large inputs.
- Raise the `go` directive to 1.25, so the `faqt` binary sizes `GOMAXPROCS` from the cgroup CPU limit rather than the number of cores on the host. The Go runtime gates this on the main module, so a tool importing faqt gets it from its own `go` directive, not this one.

### Added
- Add `seqio.Writer.Flush`, for callers that need a record to leave the output buffer without closing the writer, such as writing into a pipe a consumer reads record by record.
- Add `bam.NewReaderWithThreads`, keeping `bam.NewReader` source compatible and serial.
- Add `seqio.WithThreads` and a `--threads` flag on `to-fasta`, `stats`, `interleave` and `to-perfect-reads`, defaulting to one. Values above one are used where the format allows it, such as BGZF blocks and zstd frames.
- Add a read and write test matrix over every input format and every compression, including BGZF, exercised by path and through stdin, alongside tests for concatenated compressed streams, misleading filename suffixes, and a guard that measures CPU use to catch compression fanning out across cores again.

## [0.9.1] - 2026-09-21

### Fixed
- Merge runs by the read each FASTQ holds rather than by its position in the run's file list, so `faqt download-reads --merge` concatenates read 1 onto read 1 whatever order a run lists its files in, and can merge runs where ENA supplies an unpaired file for only some of them. Unpaired reads merge into `<prefix>.fastq.gz` alongside `<prefix>_1.fastq.gz` and `<prefix>_2.fastq.gz`; previously any run with an unpaired file was rejected with `cannot merge 3 FASTQ files per run`.
- Stop `faqt download-reads --prefix` failing with `output prefix produced duplicate FASTQ filename` on runs where ENA lists a bare orphan-reads file alongside `_1` and `_2`, such as `SRR31209530`. The bare file was numbered by its position and renamed onto read 1; it now keeps its bare name, so the three files become `<prefix>.fastq.gz`, `<prefix>_1.fastq.gz` and `<prefix>_2.fastq.gz`.

### Changed
- Merging runs that each hold a single numbered FASTQ now writes `<prefix>_1.fastq.gz` instead of `<prefix>.fastq.gz`, keeping the read number the input files carry. Runs each holding a single unpaired FASTQ are unchanged and still merge into `<prefix>.fastq.gz`.

## [0.9.0] - 2026-09-17

### Added
- Add the `agc` package for opening AGC v3 archives, listing samples, reading one sample's contigs as sequence records, and iterating complete samples in archive order.
- Detect AGC archives from content in `seqio.OpenPath`, keeping samples adjacent and prefixing flattened contig names with `sample.`.
- Add `faqt to-fasta --sample` for extracting one AGC sample with its original contig names.
- Add per-sample AGC statistics by default and `faqt stats --combine-inputs` for combining records across multiple ordinary inputs and AGC samples into one result.
- Add the `seqio.ErrEmptyInput` sentinel error and `seqio.OpenPathAllowEmpty` for library callers for which an input holding no records is a valid answer.

### Security
- Reject FASTQ filenames from ENA metadata that contain a path separator or resolve to `.`/`..` in `faqt download-reads`, preventing a maliciously crafted ENA response from writing outside the output directory.

### Fixed
- Make the `phylip` reader error on interleaved PHYLIP input instead of silently splicing a later taxon's name and sequence into the current record; only sequential PHYLIP is supported.
- Report all-zero statistics for an empty input in `faqt stats` instead of failing with `empty input`, so an assembly or bin FASTA with no contigs counts as zero sequences.

## [0.8.0] - 2026-08-14

### Added
- Add `download-reads --ena-meta-single-object` for non-merged metadata consumers that require a top-level JSON object.

### Changed
- Write ENA read metadata as a JSON list by default and combine metadata from merged runs into one ordered list.

## [0.7.0] - 2026-08-07

### Added
- Add `faqt download-reads --merge` to concatenate FASTQ files from multiple runs into one single-end output or paired `_1` and `_2` outputs; per-run FASTQs are removed after merging unless `--keep-originals` is passed.
- Add context-aware and FASTA-only `genomedl` helpers for library callers.
- Add `faqt update` to check the latest GitHub release, verify the matching archive checksum, and replace the installed binary when a newer release is available.

## [0.6.0] - 2026-07-03

### Added
- Add `faqt download --format` for genome downloads with `auto`, `fasta`, `gff3`, `genbank`, and `embl` choices, including `gb`/`gbk`/`gbff` aliases and ENA EMBL downloads.

### Changed
- Make `faqt --version` print `faqt X.Y.Z`, normalizing release tags like `vX.Y.Z` for display.
- Deprecate `faqt download --fasta` in favor of `--format fasta`.

### Fixed
- Apply the default unwrapped FASTA output behavior to genome downloads.

## [0.5.0] - 2026-06-10

### Added
- Allow `faqt download-reads` to accept comma-separated run accession lists or `--accessions-file` with one run accession per line.
- Use `PREFIX_RUN_ACCESSION` output prefixes when `faqt download-reads --prefix` is used with multiple run accessions.

## [0.4.0] - 2026-06-10

### Added
- Add `faqt download-reads` for downloading run FASTQ files from ENA or `sracha`.
- Add `readdl` package for ENA read metadata-driven FASTQ downloads with MD5 and gzip validation.
- Add configurable randomized retry delays for read FASTQ download attempts, defaulting to 5-20 seconds.
- Add `faqt download-reads --prefix` for custom FASTQ output filename prefixes.
- Add `faqt download-reads --ena-meta` for optional ENA metadata JSON output.
- Add `faqt download-reads --verbose` for progress reporting to stderr.
- Add rate-limited direct ENA byte progress to `faqt download-reads --verbose`.
- Add `faqt download-reads` options for `sracha` threads and connections.
- Add `faqt download-reads --download-stall-timeout` for progress-based direct ENA download timeouts.
- Allow `faqt download-reads` to reuse existing output directories when target files do not already exist.

### Fixed
- Make `faqt download-reads` fail before retrying or downloading when output files already exist.
- Write `faqt download-reads` outputs directly under `--output-dir` instead of creating a run accession subdirectory.
- Use a single ENA `ALL` query for `faqt download-reads --ena-meta` instead of querying ENA separately for FASTQ URLs and metadata.
- Include the exact `sracha` command in `faqt download-reads --verbose` output.
- Remove the fixed whole-file timeout from direct ENA FASTQ downloads; large files can continue while bytes are arriving.
- Choose `sracha` split mode from the ENA FASTQ manifest so single-end and unpaired reads use ENA-compatible output filenames.

## [0.3.0] - 2026-06-09

### Added
- Add unified `faqt download` command for genome assembly downloads and accession FASTA downloads.
- Add `faqt download --fasta` to force genomic FASTA downloads for genome assembly accessions.
- Add `faqt download` support for protein and nucleotide sequence accessions through NCBI EFetch.
- Add `faqt download` support for INSDC GenBank/ENA/DDBJ nucleotide and protein accession formats.
- Add `faqt download` support for WGS/TSA/TLS master accessions by expanding them to component contig/scaffold FASTA records.
- Add `faqt download --nucleotide`, `--source`, and `--assembly` for downloading CDS nucleotide sequences linked from protein accessions.
- Add `seqdl` package for sequence accession downloads from NCBI EFetch.
- Warn when `faqt download` writes genome content whose biological format conflicts with the output path suffix.
- Release builds now write a SHA-256 checksum file for packaged binary artifacts.

### Changed
- Replace separate download CLI commands with accession-based routing in `faqt download`.
- Clarify CLI `--wrap` help text to state that the default `0` disables wrapping.

### Fixed
- Apply extension-based output compression to genome downloads.
- Let genome downloads fall back to FASTA output when no annotation file is available.

### Removed
- Remove `faqt download-genome` and `faqt download-seq` as root commands.

## [0.2.1] - 2026-05-27

### Added
- Add `seqio.ReadAll` and name-keyed variants for loading sequence records into memory.
- Add genetic-code-aware translation helpers for NCBI codes 1, 4, and 11.
- Add `orf.MakeIntoGene` for Fastaq-compatible gene normalization across strands and reading frames.

## [0.2.0] - 2026-05-22

### Added
- Add configurable `genomedl.Downloader` support for custom genome download clients and endpoints.
- Add `seqio.Interleave` and `seqio.InterleavePath` for streaming interleaving of paired sequence files.
- Add `faqt interleave`, which alternates records from two input files and supports optional mate suffixes.
- Add `randomcontigs` library support for generating random FASTA contigs.
- Add `faqt make-random-contigs`, which writes random FASTA contigs to stdout by default or to `-o/--output`.
- Add half-open `seq.Interval` helper methods for validation, length, containment, intersection, distance, merging, and length sums.
- Add `seqio.CountRecords` and `seqio.CountRecordsPath` for counting records in supported sequence files.
- Add `seq.TranslateCodon` and `seq.Translate` for standard genetic-code translation.

### Changed
- Share close-error handling across CLI and library helpers.
- Share `seqio` reader setup between path-based and stream-based inputs.
- Split genome download archive and output-combining helpers by responsibility.
- Split alignment internals into focused API, anchor, DP, Smith-Waterman, and result helper files.
- Split mapper and minimizer internals into smaller files by responsibility.
- Share CLI test helpers for stdin and stdout capture.
- Reduce FASTQ reader allocations by reusing owned line buffers for sequence and quality data.
- Reduce FASTQ header parsing allocations by using borrowed line buffers on the reader hot path.
- `faqt stats` now reads from stdin when no input files are provided.
- Genome download GFF3/FASTA combination now streams files instead of reading both fully into memory.

### Fixed
- Avoid hangs and panics in paired perfect-read generation for invalid or edge-case insert sizes.
- Return an error for GFF3 inputs with `##FASTA` but no sequence records.
- Close wrapped `seqio.OpenReader` sources correctly without closing stdin for `-`.
- Preserve multi-line descriptions from GenBank and EMBL input records.
- Propagate output writer close errors from path-based conversion and read-generation helpers.
- Propagate remaining close errors from path helpers and genome download file writes.
- Support wrapped relaxed sequential PHYLIP records.
- Report non-404 GFF3 download failures instead of silently producing FASTA-only output.

## [0.1.1] - 2026-04-10

### Changed
- Release builds now produce compressed artifacts: `.tar.gz` for darwin and linux, and `.zip` for windows.

### Fixed
- FASTQ format detection now handles valid inputs whose first record is longer than the default `bufio.Reader` size, including gzipped files opened through `seqio`.

## [0.1.0] - 2026-03-28

Release `v0.1.0`, before changelog tracking started in this file.

[Unreleased]: https://github.com/martinghunt/faqt/compare/v0.9.1...HEAD
[0.9.1]: https://github.com/martinghunt/faqt/compare/v0.9.0...v0.9.1
[0.9.0]: https://github.com/martinghunt/faqt/compare/v0.8.0...v0.9.0
[0.8.0]: https://github.com/martinghunt/faqt/compare/v0.7.0...v0.8.0
[0.7.0]: https://github.com/martinghunt/faqt/compare/v0.6.0...v0.7.0
[0.6.0]: https://github.com/martinghunt/faqt/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/martinghunt/faqt/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/martinghunt/faqt/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/martinghunt/faqt/compare/v0.2.1...v0.3.0
[0.2.1]: https://github.com/martinghunt/faqt/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/martinghunt/faqt/compare/v0.1.1...v0.2.0
[0.1.1]: https://github.com/martinghunt/faqt/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/martinghunt/faqt/releases/tag/v0.1.0
