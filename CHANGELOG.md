# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed
- Release builds now produce compressed artifacts: `.tar.gz` for darwin and linux, and `.zip` for windows.

### Fixed
- FASTQ format detection now handles valid inputs whose first record is longer than the default `bufio.Reader` size, including gzipped files opened through `seqio`.

## [0.1.0] - 2026-03-28

Release `v0.1.0`, before changelog tracking started in this file.

[Unreleased]: https://github.com/martinghunt/faqt/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/martinghunt/faqt/releases/tag/v0.1.0
