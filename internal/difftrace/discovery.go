package difftrace

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// BenchPair contains an auto-discovered Go/Python trace pair plus siblings.
type BenchPair struct {
	Stem       string
	Left       string
	Right      string
	LeftRaw    string
	RightRaw   string
	LeftCSV    string
	RightCSV   string
	LeftMTime  time.Time
	RightMTime time.Time
}

type candidateTrace struct {
	path  string
	mtime time.Time
}

type benchGroup struct {
	stem   string
	goPerf []candidateTrace
	pyPerf []candidateTrace
	goRaw  []candidateTrace
	pyRaw  []candidateTrace
}

var (
	reGoPerf = regexp.MustCompile(`^(.*)_Go[^_]*_.*-perfdata\.gputrace$`)
	rePyPerf = regexp.MustCompile(`^(.*)_Python[^_]*_.*-perfdata\.gputrace$`)
	reGoRaw  = regexp.MustCompile(`^(.*)_Go[^_]*_.*\.gputrace$`)
	rePyRaw  = regexp.MustCompile(`^(.*)_Python[^_]*_.*\.gputrace$`)
)

// DiscoverBenchPair discovers the newest Go/Python trace pair by benchmark stem in dir.
func DiscoverBenchPair(dir string) (BenchPair, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return BenchPair{}, fmt.Errorf("read bench dir: %w", err)
	}

	groups := map[string]*benchGroup{}
	for _, entry := range entries {
		name := entry.Name()
		full := filepath.Join(dir, name)
		if entry.IsDir() {
			stem, side, kind, ok := classifyTraceDir(name)
			if !ok {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				continue
			}
			g := groups[stem]
			if g == nil {
				g = &benchGroup{stem: stem}
				groups[stem] = g
			}
			addCandidate(g, side, kind, candidateTrace{path: full, mtime: info.ModTime()})
		}
	}

	var pairs []BenchPair
	for _, g := range groups {
		pair, ok := selectPairFromGroup(g, dir)
		if ok {
			pairs = append(pairs, pair)
		}
	}
	if len(pairs) == 0 {
		return BenchPair{}, fmt.Errorf("no Go/Python trace pair found in %s", dir)
	}

	sort.Slice(pairs, func(i, j int) bool {
		iTime := maxTime(pairs[i].LeftMTime, pairs[i].RightMTime)
		jTime := maxTime(pairs[j].LeftMTime, pairs[j].RightMTime)
		if iTime.Equal(jTime) {
			return pairs[i].Stem < pairs[j].Stem
		}
		return iTime.After(jTime)
	})
	return pairs[0], nil
}

func classifyTraceDir(name string) (stem, side, kind string, ok bool) {
	if !strings.HasSuffix(name, ".gputrace") {
		return "", "", "", false
	}
	if m := reGoPerf.FindStringSubmatch(name); len(m) == 2 {
		return m[1], "go", "perf", true
	}
	if m := rePyPerf.FindStringSubmatch(name); len(m) == 2 {
		return m[1], "py", "perf", true
	}
	if strings.Contains(name, "-perfdata.gputrace") {
		return "", "", "", false
	}
	if m := reGoRaw.FindStringSubmatch(name); len(m) == 2 {
		return m[1], "go", "raw", true
	}
	if m := rePyRaw.FindStringSubmatch(name); len(m) == 2 {
		return m[1], "py", "raw", true
	}
	return "", "", "", false
}

func addCandidate(g *benchGroup, side, kind string, c candidateTrace) {
	switch {
	case side == "go" && kind == "perf":
		g.goPerf = append(g.goPerf, c)
	case side == "py" && kind == "perf":
		g.pyPerf = append(g.pyPerf, c)
	case side == "go" && kind == "raw":
		g.goRaw = append(g.goRaw, c)
	case side == "py" && kind == "raw":
		g.pyRaw = append(g.pyRaw, c)
	}
}

func newest(cands []candidateTrace) (candidateTrace, bool) {
	if len(cands) == 0 {
		return candidateTrace{}, false
	}
	best := cands[0]
	for _, c := range cands[1:] {
		if c.mtime.After(best.mtime) {
			best = c
		}
	}
	return best, true
}

func selectPairFromGroup(g *benchGroup, dir string) (BenchPair, bool) {
	goPerf, okGoPerf := newest(g.goPerf)
	pyPerf, okPyPerf := newest(g.pyPerf)
	goRaw, okGoRaw := newest(g.goRaw)
	pyRaw, okPyRaw := newest(g.pyRaw)

	pair := BenchPair{Stem: g.stem}
	if okGoPerf && okPyPerf {
		pair.Left, pair.Right = goPerf.path, pyPerf.path
		pair.LeftMTime, pair.RightMTime = goPerf.mtime, pyPerf.mtime
	} else if okGoRaw && okPyRaw {
		pair.Left, pair.Right = goRaw.path, pyRaw.path
		pair.LeftMTime, pair.RightMTime = goRaw.mtime, pyRaw.mtime
	} else {
		return BenchPair{}, false
	}
	if okGoRaw {
		pair.LeftRaw = goRaw.path
	}
	if okPyRaw {
		pair.RightRaw = pyRaw.path
	}
	pair.LeftCSV = findSiblingCSV(dir, pair.Left)
	pair.RightCSV = findSiblingCSV(dir, pair.Right)
	return pair, true
}

func findSiblingCSV(dir, tracePath string) string {
	base := filepath.Base(tracePath)
	name := strings.TrimSuffix(base, ".gputrace")
	name = strings.TrimSuffix(name, "-perfdata")
	csv := filepath.Join(dir, name+"_counters.csv")
	if _, err := os.Stat(csv); err == nil {
		return csv
	}
	return ""
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}
