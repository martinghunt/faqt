# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed
- Make `faqt --version` print `faqt X.Y.Z`, normalizing release tags like `vX.Y.Z` for display.

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

[Unreleased]: https://github.com/martinghunt/faqt/compare/v0.5.0...HEAD
[0.5.0]: https://github.com/martinghunt/faqt/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/martinghunt/faqt/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/martinghunt/faqt/compare/v0.2.1...v0.3.0
[0.2.1]: https://github.com/martinghunt/faqt/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/martinghunt/faqt/compare/v0.1.1...v0.2.0
[0.1.1]: https://github.com/martinghunt/faqt/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/martinghunt/faqt/releases/tag/v0.1.0
