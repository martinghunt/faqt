package seqdl

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/martinghunt/faqt/seqio"
)

func TestDownloadAccessionInfersProteinForWPAccession(t *testing.T) {
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		if got := r.URL.Query().Get("db"); got != "protein" {
			t.Fatalf("db = %q, want protein", got)
		}
		if got := r.URL.Query().Get("id"); got != "WP_002248791.1" {
			t.Fatalf("id = %q, want WP_002248791.1", got)
		}
		if got := r.URL.Query().Get("rettype"); got != "fasta" {
			t.Fatalf("rettype = %q, want fasta", got)
		}
		if got := r.URL.Query().Get("retmode"); got != "text" {
			t.Fatalf("retmode = %q, want text", got)
		}
		if got := r.URL.Query().Get("tool"); got != "faqt" {
			t.Fatalf("tool = %q, want faqt", got)
		}
		_, _ = w.Write([]byte(">WP_002248791.1 hypothetical protein\nMKKLL\n"))
	}))
	defer server.Close()

	downloader := NewDownloader()
	downloader.EFetchURL = server.URL

	outPath := filepath.Join(t.TempDir(), "protein.fa")
	err := downloader.DownloadAccessions([]string{"WP_002248791.1"}, outPath, DownloadOptions{})
	if err != nil {
		t.Fatalf("DownloadAccessions() error = %v", err)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	want := ">WP_002248791.1 hypothetical protein\nMKKLL\n"
	if string(data) != want {
		t.Fatalf("output = %q, want %q", string(data), want)
	}
	if strings.Contains(gotQuery, "api_key") {
		t.Fatalf("query unexpectedly contains api_key: %s", gotQuery)
	}
}

func TestDownloadAccessionInfersNuccoreForINSDCNucleotideAccession(t *testing.T) {
	tests := []string{"U49845.1", "AF086833.2", "AB12345678.1"}
	for _, accession := range tests {
		t.Run(accession, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := r.URL.Query().Get("db"); got != "nuccore" {
					t.Fatalf("db = %q, want nuccore", got)
				}
				if got := r.URL.Query().Get("id"); got != accession {
					t.Fatalf("id = %q, want %s", got, accession)
				}
				_, _ = fmt.Fprintf(w, ">%s test nucleotide\nACGT\n", accession)
			}))
			defer server.Close()

			downloader := NewDownloader()
			downloader.EFetchURL = server.URL

			outPath := filepath.Join(t.TempDir(), "insdc.fa")
			err := downloader.DownloadAccessions([]string{accession}, outPath, DownloadOptions{})
			if err != nil {
				t.Fatalf("DownloadAccessions() error = %v", err)
			}
			data, err := os.ReadFile(outPath)
			if err != nil {
				t.Fatalf("ReadFile() error = %v", err)
			}
			want := fmt.Sprintf(">%s test nucleotide\nACGT\n", accession)
			if string(data) != want {
				t.Fatalf("output = %q, want %q", string(data), want)
			}
		})
	}
}

func TestDownloadAccessionInfersProteinForINSDCProteinAccession(t *testing.T) {
	tests := []string{"AAA98665.1", "ABC1234567.1"}
	for _, accession := range tests {
		t.Run(accession, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := r.URL.Query().Get("db"); got != "protein" {
					t.Fatalf("db = %q, want protein", got)
				}
				if got := r.URL.Query().Get("id"); got != accession {
					t.Fatalf("id = %q, want %s", got, accession)
				}
				_, _ = fmt.Fprintf(w, ">%s test protein\nMKT\n", accession)
			}))
			defer server.Close()

			downloader := NewDownloader()
			downloader.EFetchURL = server.URL

			outPath := filepath.Join(t.TempDir(), "insdc-protein.fa")
			err := downloader.DownloadAccessions([]string{accession}, outPath, DownloadOptions{})
			if err != nil {
				t.Fatalf("DownloadAccessions() error = %v", err)
			}
			data, err := os.ReadFile(outPath)
			if err != nil {
				t.Fatalf("ReadFile() error = %v", err)
			}
			want := fmt.Sprintf(">%s test protein\nMKT\n", accession)
			if string(data) != want {
				t.Fatalf("output = %q, want %q", string(data), want)
			}
		})
	}
}

func TestDownloadAccessionsUsesRequestedDatabaseAndOptions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("db"); got != "nuccore" {
			t.Fatalf("db = %q, want nuccore", got)
		}
		if got := r.URL.Query().Get("id"); got != "NC_000001.1,NM_000002.2" {
			t.Fatalf("id = %q, want joined accessions", got)
		}
		if got := r.URL.Query().Get("api_key"); got != "key123" {
			t.Fatalf("api_key = %q, want key123", got)
		}
		if got := r.URL.Query().Get("email"); got != "user@example.org" {
			t.Fatalf("email = %q, want user@example.org", got)
		}
		_, _ = w.Write([]byte(">NC_000001.1\nACGTAC\n>NM_000002.2\nGGGG\n"))
	}))
	defer server.Close()

	downloader := NewDownloader()
	downloader.EFetchURL = server.URL

	outPath := filepath.Join(t.TempDir(), "nuccore.fa")
	err := downloader.DownloadAccessions(
		[]string{"NC_000001.1", "NM_000002.2"},
		outPath,
		DownloadOptions{
			Database: DatabaseNucleotide,
			APIKey:   "key123",
			Email:    "user@example.org",
			WriterOptions: []seqio.Option{
				seqio.WithWrap(3),
			},
		},
	)
	if err != nil {
		t.Fatalf("DownloadAccessions() error = %v", err)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	want := ">NC_000001.1\nACG\nTAC\n>NM_000002.2\nGGG\nG\n"
	if string(data) != want {
		t.Fatalf("output = %q, want %q", string(data), want)
	}
}

func TestDownloadWGSProjectAccessionWritesComponentFASTA(t *testing.T) {
	var (
		sawMasterRequest bool
		componentIDs     string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		switch query.Get("rettype") {
		case "gbc":
			sawMasterRequest = true
			if got := query.Get("db"); got != "nuccore" {
				t.Fatalf("master db = %q, want nuccore", got)
			}
			if got := query.Get("id"); got != "JABRPF000000000.1" {
				t.Fatalf("master id = %q, want JABRPF000000000.1", got)
			}
			if got := query.Get("retmode"); got != "xml" {
				t.Fatalf("master retmode = %q, want xml", got)
			}
			_, _ = w.Write([]byte(strings.Join([]string{
				`<?xml version="1.0" encoding="UTF-8"?>`,
				`<INSDSet><INSDSeq>`,
				`<INSDSeq_locus>JABRPF010000000</INSDSeq_locus>`,
				`<INSDSeq_length>3</INSDSeq_length>`,
				`<INSDSeq_alt-seq><INSDAltSeqData><INSDAltSeqData_items><INSDAltSeqItem>`,
				`<INSDAltSeqItem_first-accn>JABRPF010000001</INSDAltSeqItem_first-accn>`,
				`<INSDAltSeqItem_last-accn>JABRPF010000003</INSDAltSeqItem_last-accn>`,
				`</INSDAltSeqItem></INSDAltSeqData_items></INSDAltSeqData></INSDSeq_alt-seq>`,
				`</INSDSeq></INSDSet>`,
			}, "")))
		case "fasta":
			componentIDs = query.Get("id")
			if got := query.Get("db"); got != "nuccore" {
				t.Fatalf("component db = %q, want nuccore", got)
			}
			if componentIDs != "JABRPF010000001,JABRPF010000002,JABRPF010000003" {
				t.Fatalf("component ids = %q, want expanded component accessions", componentIDs)
			}
			_, _ = w.Write([]byte(strings.Join([]string{
				">JABRPF010000001.1 contig 1",
				"ACGT",
				">JABRPF010000002.1 contig 2",
				"GGGG",
				">JABRPF010000003.1 contig 3",
				"TTAA",
				"",
			}, "\n")))
		default:
			t.Fatalf("unexpected request query: %s", r.URL.RawQuery)
		}
	}))
	defer server.Close()

	downloader := NewDownloader()
	downloader.EFetchURL = server.URL

	outPath := filepath.Join(t.TempDir(), "wgs.fa")
	err := downloader.DownloadAccessions([]string{"JABRPF000000000.1"}, outPath, DownloadOptions{})
	if err != nil {
		t.Fatalf("DownloadAccessions() error = %v", err)
	}
	if !sawMasterRequest {
		t.Fatal("master XML request was not made")
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	want := strings.Join([]string{
		">JABRPF010000001.1 contig 1",
		"ACGT",
		">JABRPF010000002.1 contig 2",
		"GGGG",
		">JABRPF010000003.1 contig 3",
		"TTAA",
		"",
	}, "\n")
	if string(data) != want {
		t.Fatalf("output = %q, want %q", string(data), want)
	}
}

func TestDownloadWGSProjectAccessionFallsBackToLocusAndLength(t *testing.T) {
	var componentIDs string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		switch query.Get("rettype") {
		case "gbc":
			_, _ = w.Write([]byte(strings.Join([]string{
				`<?xml version="1.0" encoding="UTF-8"?>`,
				`<INSDSet><INSDSeq>`,
				`<INSDSeq_locus>AGQU01000000</INSDSeq_locus>`,
				`<INSDSeq_length>2</INSDSeq_length>`,
				`</INSDSeq></INSDSet>`,
			}, "")))
		case "fasta":
			componentIDs = query.Get("id")
			_, _ = w.Write([]byte(">AGQU01000001.1 contig 1\nACGT\n>AGQU01000002.1 contig 2\nGGGG\n"))
		default:
			t.Fatalf("unexpected request query: %s", r.URL.RawQuery)
		}
	}))
	defer server.Close()

	downloader := NewDownloader()
	downloader.EFetchURL = server.URL

	outPath := filepath.Join(t.TempDir(), "wgs.fa")
	err := downloader.DownloadAccessions([]string{"AGQU00000000.1"}, outPath, DownloadOptions{})
	if err != nil {
		t.Fatalf("DownloadAccessions() error = %v", err)
	}
	if componentIDs != "AGQU01000001,AGQU01000002" {
		t.Fatalf("component ids = %q, want fallback range", componentIDs)
	}
}

func TestDownloadAccessionNucleotideFirstUsesFirstRefSeqIPGRow(t *testing.T) {
	var regionRequests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		switch query.Get("rettype") {
		case "ipg":
			if got := query.Get("db"); got != "protein" {
				t.Fatalf("IPG db = %q, want protein", got)
			}
			if got := query.Get("id"); got != "WP_002248791.1" {
				t.Fatalf("IPG id = %q, want WP_002248791.1", got)
			}
			_, _ = w.Write([]byte(strings.Join([]string{
				"Id\tSource\tNucleotide Accession\tStart\tStop\tStrand\tProtein\tProtein Name\tOrganism\tStrain\tAssembly",
				"1\tRefSeq\tNC_000001.1\t10\t18\t+\tWP_002248791.1\tPorB\tNeisseria meningitidis\tstrain1\tGCF_1",
				"1\tRefSeq\tNC_000002.1\t20\t28\t-\tWP_002248791.1\tPorB\tNeisseria meningitidis\tstrain2\tGCF_2",
				"1\tINSDC\tCP000003.1\t30\t38\t+\tABC123.1\tPorB\tNeisseria meningitidis\tstrain3\tGCA_3",
				"",
			}, "\n")))
		case "fasta":
			regionRequests = append(regionRequests, r.URL.RawQuery)
			if got := query.Get("db"); got != "nuccore" {
				t.Fatalf("region db = %q, want nuccore", got)
			}
			if got := query.Get("id"); got != "NC_000001.1" {
				t.Fatalf("region id = %q, want NC_000001.1", got)
			}
			if got := query.Get("seq_start"); got != "10" {
				t.Fatalf("seq_start = %q, want 10", got)
			}
			if got := query.Get("seq_stop"); got != "18" {
				t.Fatalf("seq_stop = %q, want 18", got)
			}
			if got := query.Get("strand"); got != "1" {
				t.Fatalf("strand = %q, want 1", got)
			}
			_, _ = w.Write([]byte(">NC_000001.1:10-18 coding sequence\nATGAAATAA\n"))
		default:
			t.Fatalf("unexpected request query: %s", r.URL.RawQuery)
		}
	}))
	defer server.Close()

	downloader := NewDownloader()
	downloader.EFetchURL = server.URL

	outPath := filepath.Join(t.TempDir(), "cds.fa")
	err := downloader.DownloadAccessions(
		[]string{"WP_002248791.1"},
		outPath,
		DownloadOptions{Nucleotide: NucleotideFirst},
	)
	if err != nil {
		t.Fatalf("DownloadAccessions() error = %v", err)
	}
	if len(regionRequests) != 1 {
		t.Fatalf("region requests = %d, want 1", len(regionRequests))
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	want := ">NC_000001.1:10-18 coding sequence\nATGAAATAA\n"
	if string(data) != want {
		t.Fatalf("output = %q, want %q", string(data), want)
	}
}

func TestDownloadAccessionNucleotideAllFiltersBySourceAndAssembly(t *testing.T) {
	var regionRequests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		switch query.Get("rettype") {
		case "ipg":
			_, _ = w.Write([]byte(strings.Join([]string{
				"Id\tSource\tNucleotide Accession\tStart\tStop\tStrand\tProtein\tProtein Name\tOrganism\tStrain\tAssembly",
				"1\tRefSeq\tNC_000001.1\t10\t18\t+\tWP_002248791.1\tPorB\tNeisseria meningitidis\tstrain1\tGCF_1",
				"1\tINSDC\tCP000002.1\t20\t28\t-\tABC123.1\tPorB\tNeisseria meningitidis\tstrain2\tGCA_2",
				"1\tINSDC\tCP000003.1\t30\t38\t+\tABC124.1\tPorB\tNeisseria meningitidis\tstrain3\tGCA_3",
				"",
			}, "\n")))
		case "fasta":
			regionRequests = append(regionRequests, r.URL.RawQuery)
			if got := query.Get("id"); got != "CP000002.1" {
				t.Fatalf("region id = %q, want CP000002.1", got)
			}
			if got := query.Get("seq_start"); got != "20" {
				t.Fatalf("seq_start = %q, want 20", got)
			}
			if got := query.Get("seq_stop"); got != "28" {
				t.Fatalf("seq_stop = %q, want 28", got)
			}
			if got := query.Get("strand"); got != "2" {
				t.Fatalf("strand = %q, want 2", got)
			}
			_, _ = w.Write([]byte(">CP000002.1:c28-20 coding sequence\nTTATTTCAT\n"))
		default:
			t.Fatalf("unexpected request query: %s", r.URL.RawQuery)
		}
	}))
	defer server.Close()

	downloader := NewDownloader()
	downloader.EFetchURL = server.URL

	outPath := filepath.Join(t.TempDir(), "cds.fa")
	err := downloader.DownloadAccessions(
		[]string{"WP_002248791.1"},
		outPath,
		DownloadOptions{
			Nucleotide: NucleotideAll,
			Source:     SourceAll,
			Assembly:   "GCA_2",
		},
	)
	if err != nil {
		t.Fatalf("DownloadAccessions() error = %v", err)
	}
	if len(regionRequests) != 1 {
		t.Fatalf("region requests = %d, want 1", len(regionRequests))
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	want := ">CP000002.1:c28-20 coding sequence\nTTATTTCAT\n"
	if string(data) != want {
		t.Fatalf("output = %q, want %q", string(data), want)
	}
}

func TestDownloadAccessionNucleotideAllWritesAllMatchingRows(t *testing.T) {
	var regionCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		switch query.Get("rettype") {
		case "ipg":
			_, _ = w.Write([]byte(strings.Join([]string{
				"Id\tSource\tNucleotide Accession\tStart\tStop\tStrand\tProtein\tProtein Name\tOrganism\tStrain\tAssembly",
				"1\tRefSeq\tNC_000001.1\t10\t18\t+\tWP_002248791.1\tPorB\tNeisseria meningitidis\tstrain1\tGCF_1",
				"1\tRefSeq\tNC_000002.1\t20\t28\t+\tWP_002248791.1\tPorB\tNeisseria meningitidis\tstrain2\tGCF_2",
				"1\tINSDC\tCP000003.1\t30\t38\t+\tABC124.1\tPorB\tNeisseria meningitidis\tstrain3\tGCA_3",
				"",
			}, "\n")))
		case "fasta":
			regionCount++
			_, _ = fmt.Fprintf(w, ">%s:%s-%s\nATGAAATAA\n", query.Get("id"), query.Get("seq_start"), query.Get("seq_stop"))
		default:
			t.Fatalf("unexpected request query: %s", r.URL.RawQuery)
		}
	}))
	defer server.Close()

	downloader := NewDownloader()
	downloader.EFetchURL = server.URL

	outPath := filepath.Join(t.TempDir(), "cds.fa")
	err := downloader.DownloadAccessions(
		[]string{"WP_002248791.1"},
		outPath,
		DownloadOptions{Nucleotide: NucleotideAll},
	)
	if err != nil {
		t.Fatalf("DownloadAccessions() error = %v", err)
	}
	if regionCount != 2 {
		t.Fatalf("region requests = %d, want 2 RefSeq rows", regionCount)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	for _, want := range []string{">NC_000001.1:10-18", ">NC_000002.1:20-28"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("output = %q, missing %q", string(data), want)
		}
	}
	if strings.Contains(string(data), "CP000003.1") {
		t.Fatalf("output includes INSDC row despite default RefSeq source: %q", string(data))
	}
}

func TestDownloadAccessionsRejectsUnsupportedDatabase(t *testing.T) {
	err := NewDownloader().DownloadAccessions(
		[]string{"WP_002248791.1"},
		filepath.Join(t.TempDir(), "out.fa"),
		DownloadOptions{Database: Database("bad")},
	)
	if err == nil || !strings.Contains(err.Error(), "unsupported database") {
		t.Fatalf("DownloadAccessions() error = %v, want unsupported database", err)
	}
}

func TestDownloadAccessionsRejectsUnsupportedNucleotideMode(t *testing.T) {
	err := NewDownloader().DownloadAccessions(
		[]string{"WP_002248791.1"},
		filepath.Join(t.TempDir(), "out.fa"),
		DownloadOptions{Nucleotide: NucleotideMode("bad")},
	)
	if err == nil || !strings.Contains(err.Error(), "unsupported nucleotide mode") {
		t.Fatalf("DownloadAccessions() error = %v, want unsupported nucleotide mode", err)
	}
}

func TestDownloadAccessionsRejectsUnsupportedSource(t *testing.T) {
	err := NewDownloader().DownloadAccessions(
		[]string{"WP_002248791.1"},
		filepath.Join(t.TempDir(), "out.fa"),
		DownloadOptions{Nucleotide: NucleotideFirst, Source: Source("bad")},
	)
	if err == nil || !strings.Contains(err.Error(), "unsupported source") {
		t.Fatalf("DownloadAccessions() error = %v, want unsupported source", err)
	}
}

func TestDownloadAccessionsReportsNotFound(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()

	downloader := NewDownloader()
	downloader.EFetchURL = server.URL

	err := downloader.DownloadAccessions(
		[]string{"WP_002248791.1"},
		filepath.Join(t.TempDir(), "out.fa"),
		DownloadOptions{},
	)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("DownloadAccessions() error = %v, want os.ErrNotExist", err)
	}
}

func TestDownloadAccessionsRejectsNonSequenceResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html>not fasta</html>"))
	}))
	defer server.Close()

	downloader := NewDownloader()
	downloader.EFetchURL = server.URL

	err := downloader.DownloadAccessions(
		[]string{"WP_002248791.1"},
		filepath.Join(t.TempDir(), "out.fa"),
		DownloadOptions{},
	)
	if err == nil || !strings.Contains(err.Error(), "not supported sequence content") {
		t.Fatalf("DownloadAccessions() error = %v, want sequence content error", err)
	}
}

func TestInferDatabaseMixedAccessionsUsesSequences(t *testing.T) {
	got := inferDatabase([]string{"WP_002248791.1", "NC_000001.1"})
	if got != DatabaseSequences {
		t.Fatalf("inferDatabase() = %q, want %q", got, DatabaseSequences)
	}
}

func TestInferDatabaseRecognizesWGSProjectAndComponentAccessions(t *testing.T) {
	tests := []string{
		"AGQU00000000.1",
		"AGQU01",
		"AGQU01000001.1",
		"JABRPF000000000.1",
		"JABRPF01",
		"JABRPF010000001.1",
	}
	for _, accession := range tests {
		t.Run(accession, func(t *testing.T) {
			got := inferDatabase([]string{accession})
			if got != DatabaseNuccore {
				t.Fatalf("inferDatabase() = %q, want %q", got, DatabaseNuccore)
			}
		})
	}
}
