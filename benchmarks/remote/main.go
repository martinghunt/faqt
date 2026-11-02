//go:build linux || darwin

// Remote is a self-contained runner for cross-compiled alignment test bundles.
package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"
)

type options struct {
	data, out, minimap    string
	runs                  int
	selfTestOnly, profile bool
	timeout               time.Duration
}
type dataset struct {
	Name, A, B, AHash, BHash, PAFHash string
	Count                             int
}
type reference struct {
	Datasets []dataset
	Source   string
}
type processResult struct {
	Name                    string
	Command                 []string
	WallSeconds, CPUSeconds float64
	PeakRSSBytes            int64
	ExitCode                int
	Error                   string `json:",omitempty"`
}
type sample struct {
	Dataset, Backend string
	Run              int
	Processes        []processResult
	PAFHash          string          `json:",omitempty"`
	Count            int             `json:",omitempty"`
	ReferenceMatch   *bool           `json:",omitempty"`
	PAFHashes        []string        `json:",omitempty"`
	Detail           json.RawMessage `json:",omitempty"`
}
type result struct {
	Success                                  bool
	Error                                    string `json:",omitempty"`
	Started, GOOS, GOARCH, GoVersion, Source string
	Runs                                     int
	BinaryHashes                             map[string]string
	Inputs                                   map[string]string
	Checks                                   []processResult
	Samples                                  []sample
	Minimap2                                 string
	Summary                                  []summaryEntry
}
type summaryEntry struct {
	Dataset, Backend      string
	Runs                  int
	WallMedian, CPUMedian float64
	PeakRSSBytes          int64
}

func main() {
	dir, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], filepath.Dir(dir), os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func parseOptions(args []string) (options, error) {
	o := options{}
	f := flag.NewFlagSet("run-tests-and-benchmarks", flag.ContinueOnError)
	f.StringVar(&o.data, "data", "", "directory containing the debug FASTA pairs")
	f.StringVar(&o.out, "out", "", "fresh output directory (default: a new directory beside the bundle)")
	f.StringVar(&o.minimap, "minimap2", "auto", "minimap2 executable, auto, or none")
	f.IntVar(&o.runs, "runs", 3, "fresh-process repetitions per engine and dataset")
	f.BoolVar(&o.selfTestOnly, "self-test-only", false, "run CPU and correctness checks without genome benchmarks")
	f.BoolVar(&o.profile, "profile", false, "collect an extra untimed SIMD CPU profile per dataset")
	f.DurationVar(&o.timeout, "timeout", 15*time.Minute, "timeout per child process")
	if err := f.Parse(args); err != nil {
		return o, err
	}
	if f.NArg() == 1 && o.data == "" {
		o.data = f.Arg(0)
	} else if f.NArg() != 0 {
		return o, errors.New("pass one data directory; put flags before the positional directory")
	}
	if o.runs < 1 || o.timeout <= 0 {
		return o, errors.New("runs and timeout must be positive")
	}
	if o.data == "" && !o.selfTestOnly {
		return o, errors.New("supply --data DIR, a positional data directory, or --self-test-only")
	}
	return o, nil
}

func run(ctx context.Context, args []string, bundle string, log io.Writer) (returned error) {
	o, err := parseOptions(args)
	if err != nil {
		return err
	}
	if o.out == "" {
		o.out, err = os.MkdirTemp(bundle, "results-")
	} else {
		err = os.Mkdir(o.out, 0755)
	}
	if err != nil {
		return fmt.Errorf("create fresh output directory: %w", err)
	}
	o.out, err = filepath.Abs(o.out)
	if err != nil {
		return err
	}
	r := result{Started: time.Now().UTC().Format(time.RFC3339), GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, GoVersion: runtime.Version(), Runs: o.runs, BinaryHashes: map[string]string{}, Inputs: map[string]string{}}
	defer func() {
		r.Success = returned == nil
		if returned != nil {
			r.Error = returned.Error()
		}
		r.Summary = summarize(r.Samples)
		for _, s := range r.Summary {
			fmt.Fprintf(log, "SUMMARY %-8s %-8s n=%d wall=%.3fs CPU=%.3fs peak=%.1fMiB\n", s.Dataset, s.Backend, s.Runs, s.WallMedian, s.CPUMedian, float64(s.PeakRSSBytes)/(1<<20))
		}
		if err := writeJSON(filepath.Join(o.out, "results.json"), r); err != nil {
			returned = errors.Join(returned, err)
		}
		archive := o.out + ".tar.gz"
		if err := packResults(o.out, archive); err != nil {
			returned = errors.Join(returned, err)
		} else {
			fmt.Fprintln(log, "RESULT ARCHIVE:", archive)
		}
		fmt.Fprintln(log, "Full logs and PAFs:", o.out)
	}()
	for _, command := range [][]string{{"lscpu"}, {"uname", "-a"}} {
		if data, err := exec.CommandContext(ctx, command[0], command[1:]...).CombinedOutput(); err == nil {
			_ = os.WriteFile(filepath.Join(o.out, command[0]+".txt"), data, 0644)
		}
	}
	if data, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		_ = os.WriteFile(filepath.Join(o.out, "cpuinfo.txt"), data, 0644)
	}
	for _, name := range []string{"align-scalar.test", "align-simd.test", "alignment-scalar.test", "alignment-simd.test", "runner"} {
		hash, err := fileHash(filepath.Join(bundle, name))
		if err != nil {
			return err
		}
		r.BinaryHashes[name] = hash
	}
	var ref reference
	data, err := os.ReadFile(filepath.Join(bundle, "reference.json"))
	if err != nil {
		return err
	}
	if err = json.Unmarshal(data, &ref); err != nil {
		return err
	}
	r.Source = ref.Source
	var selected []dataset
	if o.data != "" {
		o.data, err = filepath.Abs(o.data)
		if err != nil {
			return err
		}
		for _, d := range ref.Datasets {
			_, ea := os.Stat(filepath.Join(o.data, d.A))
			_, eb := os.Stat(filepath.Join(o.data, d.B))
			if os.IsNotExist(ea) && os.IsNotExist(eb) {
				continue
			}
			if ea != nil || eb != nil {
				return fmt.Errorf("dataset %s requires both %s and %s", d.Name, d.A, d.B)
			}
			for name, want := range map[string]string{d.A: d.AHash, d.B: d.BHash} {
				hash, err := fileHash(filepath.Join(o.data, name))
				if err != nil {
					return err
				}
				r.Inputs[name] = hash
				if hash != want {
					return fmt.Errorf("input hash differs from the reference: %s; copy the original FASTA without rewriting it", name)
				}
			}
			selected = append(selected, d)
		}
		if len(selected) == 0 {
			return errors.New("data directory contains neither complete reference pair")
		}
	}
	check := func(name, binary string, flags []string, extra map[string]string) error {
		fmt.Fprintln(log, "CHECK", name)
		p := execute(ctx, o.timeout, name, append([]string{filepath.Join(bundle, binary)}, flags...), environment(extra), o.out, "")
		r.Checks = append(r.Checks, p)
		if p.ExitCode != 0 || p.Error != "" {
			return fmt.Errorf("%s failed; see %s.stderr and %s.stdout", name, name, name)
		}
		return nil
	}
	flags := []string{"-test.run=^TestSIMDBackend$", "-test.v", "-test.timeout=" + o.timeout.String()}
	if err := check("backend-avx2", "align-simd.test", flags, map[string]string{"FAQT_EXPECT_BACKEND": "avx2-16", "FAQT_BACKEND_OUTPUT": filepath.Join(o.out, "backend.json")}); err != nil {
		return err
	}
	unitFlags := []string{"-test.v", "-test.timeout=" + o.timeout.String()}
	if err := check("unit-scalar", "align-scalar.test", unitFlags, map[string]string{"FAQT_EXPECT_BACKEND": "scalar"}); err != nil {
		return err
	}
	extra := map[string]string{"FAQT_EXPECT_BACKEND": "avx2-16"}
	for _, d := range selected {
		if d.Name == "debug" {
			extra["FAQT_SEQHIKER_DEBUG"] = o.data
		}
	}
	if err := check("unit-avx2", "align-simd.test", unitFlags, extra); err != nil {
		return err
	}
	godebug := os.Getenv("GODEBUG")
	if godebug != "" {
		godebug += ","
	}
	if err := check("fallback-no-avx2", "align-simd.test", []string{"-test.run=^Test(SIMDBackend|DPPlatform.*|DPBoundary.*|Adaptive.*)$", "-test.v"}, map[string]string{"FAQT_EXPECT_BACKEND": "scalar", "GODEBUG": godebug + "cpu.avx2=off"}); err != nil {
		return err
	}
	// Check the AVX2 path with all five core AVX-512 flags disabled as well.
	if err := check("avx2-without-avx512", "align-simd.test", []string{"-test.run=^Test(SIMDBackend|Difference.*|DPPlatform.*|DPBoundary.*|Adaptive.*)$", "-test.v"}, map[string]string{"FAQT_EXPECT_BACKEND": "avx2-16", "GODEBUG": godebug + "cpu.avx512f=off,cpu.avx512cd=off,cpu.avx512bw=off,cpu.avx512dq=off,cpu.avx512vl=off"}); err != nil {
		return err
	}
	if o.selfTestOnly {
		return nil
	}
	mismatch := false
	for _, d := range selected {
		for _, backend := range []string{"scalar", "simd"} {
			for repeat := 1; repeat <= o.runs; repeat++ {
				name := fmt.Sprintf("%s.%s.%d", d.Name, backend, repeat)
				fmt.Fprintln(log, "BENCH", name)
				stem := filepath.Join(o.out, name)
				extra := benchmarkEnv(o, d, stem)
				cmd := []string{filepath.Join(bundle, "alignment-"+backend+".test"), "-test.run=^TestComparison$", "-test.v", "-test.timeout=" + o.timeout.String()}
				p := execute(ctx, o.timeout, name, cmd, environment(extra), o.out, "")
				s := sample{Dataset: d.Name, Backend: backend, Run: repeat, Processes: []processResult{p}}
				if p.ExitCode != 0 || p.Error != "" {
					r.Samples = append(r.Samples, s)
					return fmt.Errorf("benchmark %s failed", name)
				}
				detail, err := os.ReadFile(stem + ".json")
				if err != nil {
					return err
				}
				var report struct {
					Count int
					Error string
				}
				if err = json.Unmarshal(detail, &report); err != nil {
					return err
				}
				if report.Error != "" {
					return errors.New(report.Error)
				}
				s.Detail = detail
				s.Count = report.Count
				s.PAFHash, err = fileHash(stem + ".paf")
				if err != nil {
					return err
				}
				matches := s.PAFHash == d.PAFHash && s.Count == d.Count
				s.ReferenceMatch = &matches
				mismatch = mismatch || !matches
				r.Samples = append(r.Samples, s)
				fmt.Fprintf(log, "  %.3fs wall, %.3fs CPU, %d paths, reference=%v\n", p.WallSeconds, p.CPUSeconds, s.Count, matches)
				if err := writeJSON(filepath.Join(o.out, "results.json"), r); err != nil {
					return err
				}
			}
		}
	}
	if mismatch {
		return errors.New("one or more alignments differ from the scalar reference; inspect results.json and the retained PAFs")
	}
	mm := o.minimap
	if mm == "auto" {
		mm, _ = exec.LookPath("minimap2")
	}
	if mm == "none" {
		mm = ""
	}
	if mm != "" {
		version, err := exec.CommandContext(ctx, mm, "--version").CombinedOutput()
		if err != nil {
			return fmt.Errorf("minimap2 --version: %w", err)
		}
		r.Minimap2 = strings.TrimSpace(string(version))
		for _, d := range selected {
			for repeat := 1; repeat <= o.runs; repeat++ {
				s := sample{Dataset: d.Name, Backend: "minimap2", Run: repeat}
				for direction := 0; direction < 2; direction++ {
					name := fmt.Sprintf("%s.minimap2.%d.%d", d.Name, repeat, direction)
					fmt.Fprintln(log, "BENCH", name)
					target, query := d.B, d.A
					if direction == 1 {
						target, query = d.A, d.B
					}
					cmd := append([]string{mm}, minimapFlags()...)
					cmd = append(cmd, filepath.Join(o.data, target), filepath.Join(o.data, query))
					p := execute(ctx, o.timeout, name, cmd, environment(nil), o.out, filepath.Join(o.out, name+".paf"))
					s.Processes = append(s.Processes, p)
					if p.ExitCode != 0 || p.Error != "" {
						r.Samples = append(r.Samples, s)
						return fmt.Errorf("%s failed", name)
					}
					hash, err := fileHash(filepath.Join(o.out, name+".paf"))
					if err != nil {
						return err
					}
					s.PAFHashes = append(s.PAFHashes, hash)
				}
				r.Samples = append(r.Samples, s)
			}
		}
	} else {
		r.Minimap2 = "not available or explicitly skipped"
	}
	if o.profile {
		for _, d := range selected {
			name := d.Name + ".simd-profile"
			extra := benchmarkEnv(o, d, filepath.Join(o.out, name))
			extra["FAQT_CPU_PROFILE"] = filepath.Join(o.out, name+".cpu")
			if err := check(name, "alignment-simd.test", []string{"-test.run=^TestComparison$", "-test.v"}, extra); err != nil {
				return err
			}
		}
	}
	return nil
}

func benchmarkEnv(o options, d dataset, stem string) map[string]string {
	return map[string]string{"FAQT_BENCH_A": filepath.Join(o.data, d.A), "FAQT_BENCH_B": filepath.Join(o.data, d.B), "FAQT_BENCH_OUTPUT": stem, "FAQT_MAX_ANCHORS": "20000000", "FAQT_BENCH_END_EXTENSION": "dp", "FAQT_BENCH_TIMEOUT": o.timeout.String()}
}
func minimapFlags() []string {
	return []string{"-x", "asm5", "-c", "--eqx", "-P", "-t", "1", "-k", "15", "-w", "10", "-A", "2", "-B", "4", "-O", "4,24", "-E", "2,1", "-g", "2000", "-r", "2000,2000", "-z", "200,200", "-m", "40", "-s", "40", "-n", "1", "-f", "100000000", "-U", "100000000,100000000", "--q-occ-frac", "0", "--no-long-join", "--rmq=no"}
}
func environment(extra map[string]string) []string {
	values := map[string]string{}
	for _, entry := range os.Environ() {
		k, v, _ := strings.Cut(entry, "=")
		if !strings.HasPrefix(k, "FAQT_") && k != "GOMAXPROCS" {
			values[k] = v
		}
	}
	values["GOMAXPROCS"] = "1"
	for k, v := range extra {
		values[k] = v
	}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k+"="+values[k])
	}
	return out
}
func execute(parent context.Context, timeout time.Duration, name string, args, env []string, out, stdout string) processResult {
	r := processResult{Name: name, Command: args, ExitCode: -1}
	start := time.Now()
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	if stdout == "" {
		stdout = filepath.Join(out, name+".stdout")
	}
	f, err := os.Create(stdout)
	if err != nil {
		r.Error = err.Error()
		return r
	}
	defer f.Close()
	e, err := os.Create(filepath.Join(out, name+".stderr"))
	if err != nil {
		r.Error = err.Error()
		return r
	}
	defer e.Close()
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Env = env
	cmd.Stdout = f
	cmd.Stderr = e
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		if err == syscall.ESRCH {
			return os.ErrProcessDone
		}
		return err
	}
	cmd.WaitDelay = 3 * time.Second
	err = cmd.Run()
	r.WallSeconds = time.Since(start).Seconds()
	if cmd.ProcessState != nil {
		r.ExitCode = cmd.ProcessState.ExitCode()
		r.CPUSeconds = cmd.ProcessState.UserTime().Seconds() + cmd.ProcessState.SystemTime().Seconds()
		if usage, ok := cmd.ProcessState.SysUsage().(*syscall.Rusage); ok {
			r.PeakRSSBytes = usage.Maxrss
			if runtime.GOOS == "linux" {
				r.PeakRSSBytes *= 1024
			}
		}
	}
	if err != nil {
		r.Error = err.Error()
	}
	return r
}
func fileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0644)
}

func summarize(samples []sample) []summaryEntry {
	groups := map[string][]sample{}
	for _, s := range samples {
		valid := len(s.Processes) > 0
		for _, p := range s.Processes {
			valid = valid && p.ExitCode == 0 && p.Error == ""
		}
		if valid {
			key := s.Dataset + "\x00" + s.Backend
			groups[key] = append(groups[key], s)
		}
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var out []summaryEntry
	for _, k := range keys {
		values := groups[k]
		r := summaryEntry{Dataset: values[0].Dataset, Backend: values[0].Backend, Runs: len(values)}
		var walls, cpus []float64
		for _, s := range values {
			var wall, cpu float64
			for _, p := range s.Processes {
				wall += p.WallSeconds
				cpu += p.CPUSeconds
				r.PeakRSSBytes = max(r.PeakRSSBytes, p.PeakRSSBytes)
			}
			walls = append(walls, wall)
			cpus = append(cpus, cpu)
		}
		r.WallMedian = median(walls)
		r.CPUMedian = median(cpus)
		out = append(out, r)
	}
	return out
}
func median(values []float64) float64 {
	sort.Float64s(values)
	n := len(values)
	if n%2 == 1 {
		return values[n/2]
	}
	return (values[n/2-1] + values[n/2]) / 2
}

// Return compact diagnostics. Input FASTAs and large PAFs stay on the machine.
func packResults(dir, dest string) (returned error) {
	f, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	z := gzip.NewWriter(f)
	tw := tar.NewWriter(z)
	defer func() { returned = errors.Join(returned, tw.Close(), z.Close(), f.Close()) }()
	return filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".paf" || ext == ".fa" || ext == ".fasta" {
			return nil
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		header.Name = filepath.Join(filepath.Base(dir), rel)
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(tw, input)
		return errors.Join(copyErr, input.Close())
	})
}
