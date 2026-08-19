package agc_test

import (
	"bytes"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/martinghunt/faqt/agc"
)

func TestArchiveReadsSamplesAndSelectedGenome(t *testing.T) {
	path := writeToyArchive(t, "misleading.fasta")
	archive, err := agc.OpenPath(path)
	if err != nil {
		t.Fatalf("OpenPath() error = %v", err)
	}
	defer archive.Close()

	samples, err := archive.Samples()
	if err != nil {
		t.Fatalf("Samples() error = %v", err)
	}
	if want := []string{"ref", "a", "b", "c"}; !reflect.DeepEqual(samples, want) {
		t.Fatalf("Samples() = %#v, want %#v", samples, want)
	}

	r, err := archive.OpenSample("b")
	if err != nil {
		t.Fatalf("OpenSample() error = %v", err)
	}
	var got []string
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("Read() error = %v", err)
		}
		got = append(got, rec.Name+"|"+rec.Description+"="+string(rec.Seq))
	}
	want := []string{
		"chr1|=AAAAAAAAA",
		"g|h i 21=GGGAGGG",
		"c|=CCCCCCCCC",
		"t|=TTTTTTT",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("records = %#v, want %#v", got, want)
	}
}

func TestArchiveIteratesCompleteSamplesInArchiveOrder(t *testing.T) {
	archive, err := agc.OpenPath(writeToyArchive(t, "toy.agc"))
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()

	var got []string
	err = archive.IterateSamples(func(sample string, r *agc.Reader) error {
		var contigs []string
		for {
			rec, err := r.Read()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return err
			}
			contigs = append(contigs, rec.Name)
		}
		got = append(got, sample+":"+strings.Join(contigs, ","))
		return nil
	})
	if err != nil {
		t.Fatalf("IterateSamples() error = %v", err)
	}
	want := []string{"ref:chr1,chr2,chr3,seq", "a:chr1a,chr3a", "b:chr1,g,c,t", "c:1,2,3"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sample groups = %#v, want %#v", got, want)
	}
}

func TestArchiveOpenAllPrefixesContigsAndKeepsSamplesAdjacent(t *testing.T) {
	archive, err := agc.OpenPath(writeToyArchive(t, "toy.agc"))
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	r, err := archive.OpenAll()
	if err != nil {
		t.Fatal(err)
	}

	var got []string
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, rec.Name+"|"+rec.Description)
	}
	want := []string{
		"ref.chr1|", "ref.chr2|", "ref.chr3|", "ref.seq|",
		"a.chr1a|", "a.chr3a|",
		"b.chr1|", "b.g|h i 21", "b.c|", "b.t|",
		"c.1|", "c.2|", "c.3|",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("OpenAll records = %#v, want %#v", got, want)
	}
}

func TestArchiveRejectsUnknownSampleAndNilIterationCallback(t *testing.T) {
	archive, err := agc.OpenPath(writeToyArchive(t, "toy.agc"))
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	if _, err := archive.OpenSample("missing"); !errors.Is(err, agc.ErrSampleNotFound) {
		t.Fatalf("OpenSample(missing) error = %v, want ErrSampleNotFound", err)
	}
	if err := archive.IterateSamples(nil); err == nil {
		t.Fatal("IterateSamples(nil) error = nil")
	}
}

func TestOpenPathRejectsStdinBecauseAGCRequiresRandomAccess(t *testing.T) {
	if _, err := agc.OpenPath("-"); !errors.Is(err, agc.ErrRandomAccessRequired) {
		t.Fatalf("OpenPath(-) error = %v, want ErrRandomAccessRequired", err)
	}
}

func TestOpenReaderAtLeavesCallerSourceUsable(t *testing.T) {
	data := toyArchiveData(t)
	source := bytes.NewReader(data)
	archive, err := agc.OpenReaderAt(source, int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	var first [1]byte
	if _, err := source.ReadAt(first[:], 0); err != nil {
		t.Fatalf("caller-owned ReaderAt unusable after Archive.Close(): %v", err)
	}
}

func writeToyArchive(t *testing.T, name string) string {
	t.Helper()
	data := toyArchiveData(t)
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func toyArchiveData(t *testing.T) []byte {
	t.Helper()
	encoded, err := os.ReadFile(filepath.Join("testdata", "toy_ex.agc.b64"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := base64.StdEncoding.DecodeString(string(encoded))
	if err != nil {
		t.Fatal(err)
	}
	return data
}
