package admin_metrics_gateway

import (
	"alt/domain"
	"bufio"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// prometheusConfigRelPath is the repo-relative scrape config that decides which
// jobs can ever produce an `up` sample.
const prometheusConfigRelPath = "observability/prometheus/prometheus.yml"

var (
	// Matches a scrape_configs entry, e.g. "  - job_name: 'plecto-proxy'".
	// Applied to a comment-stripped line, so a trailing YAML comment cannot
	// hide a job from the guard.
	scrapeJobRe = regexp.MustCompile(`^\s*-\s*job_name:\s*['"]?([A-Za-z0-9_.\-]+)['"]?\s*$`)
	// Matches a target list belonging to the job most recently seen, e.g.
	// "      - targets: ['pki-agent-alt-backend:9510']".
	scrapeTargetsRe = regexp.MustCompile(`^\s*-\s*targets:\s*(.*)$`)
	// Extracts the alternation inside the availability entry's job matcher.
	jobMatcherRe = regexp.MustCompile(`job=~"([^"]*)"`)
)

// scrapeJob is one active scrape_configs entry and how many targets it fans out
// to. The target count matters because every consumer of the availability tile
// keys its rows by `job` alone, so a job with several targets collapses into a
// single row.
type scrapeJob struct {
	name    string
	targets int
}

// parseScrapeJobs reads the job_name of every *active* scrape config, plus the
// number of targets each one fans out to. Commented-out jobs (currently
// pre-processor) are excluded: they emit nothing, so matching them would
// reintroduce the phantom-row problem from the other side.
func parseScrapeJobs(r io.Reader) ([]scrapeJob, error) {
	var jobs []scrapeJob
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := stripYAMLComment(sc.Text())
		if m := scrapeJobRe.FindStringSubmatch(line); m != nil {
			jobs = append(jobs, scrapeJob{name: m[1]})
			continue
		}
		if m := scrapeTargetsRe.FindStringSubmatch(line); m != nil && len(jobs) > 0 {
			jobs[len(jobs)-1].targets += countTargets(m[1])
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return jobs, nil
}

// stripYAMLComment removes a trailing `#` comment, which in YAML starts only at
// the beginning of a line or after whitespace and never inside a quoted scalar.
// A commented-out job therefore reduces to blank, and an active job carrying an
// end-of-line note stays visible to the parser.
func stripYAMLComment(line string) string {
	var quote rune
	afterSpace := true
	for i, r := range line {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			}
		case r == '\'' || r == '"':
			quote = r
		case r == '#' && afterSpace:
			return line[:i]
		}
		afterSpace = r == ' ' || r == '\t'
	}
	return line
}

// countTargets counts the hosts on the right-hand side of a `- targets:` key.
// The flow form `['a:1', 'b:2']` is what prometheus.yml uses; the block form
// (nothing after the colon) counts as one so a job is never reported as
// single-target when it is not.
func countTargets(rest string) int {
	rest = strings.TrimSpace(rest)
	if !strings.HasPrefix(rest, "[") || !strings.HasSuffix(rest, "]") {
		return 1
	}
	inner := strings.TrimSpace(rest[1 : len(rest)-1])
	if inner == "" {
		return 0
	}
	return len(strings.Split(inner, ","))
}

// findPrometheusConfig walks up from the package directory to the repo root.
func findPrometheusConfig(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		candidate := filepath.Join(dir, filepath.FromSlash(prometheusConfigRelPath))
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			// Fail rather than skip. A skip here is how this guard stayed
			// inert: the backend CI job checks out only the directories named
			// in .github/workflows/backend-go.yaml, so dropping `observability`
			// from that list would silently disable the drift check instead of
			// turning the build red.
			t.Fatalf("%s not found walking up from the package dir; add `observability` to the sparse-checkout in .github/workflows/backend-go.yaml", prometheusConfigRelPath)
		}
		dir = parent
	}
}

func prometheusScrapeJobs(t *testing.T, path string) []scrapeJob {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()

	jobs, err := parseScrapeJobs(f)
	if err != nil {
		t.Fatalf("scan %s: %v", path, err)
	}
	if len(jobs) == 0 {
		t.Fatalf("no job_name found in %s; parser is broken", path)
	}
	return jobs
}

func availabilityJobMatcher(t *testing.T) (entry allowEntry, allowed []string) {
	t.Helper()
	entry, ok := lookup(domain.MetricAvailability)
	if !ok {
		t.Fatalf("allowlist has no %q entry", domain.MetricAvailability)
	}
	m := jobMatcherRe.FindStringSubmatch(entry.promql)
	if m == nil {
		t.Fatalf(`availability promql has no job=~"..." matcher: %s`, entry.promql)
	}
	return entry, strings.Split(m[1], "|")
}

func TestStripYAMLComment(t *testing.T) {
	tests := []struct {
		name string
		line string
		want string
	}{
		{name: "no comment", line: "  - job_name: 'alt-backend'", want: "  - job_name: 'alt-backend'"},
		{name: "trailing comment", line: "  - job_name: 'alt-backend'  # core API", want: "  - job_name: 'alt-backend'  "},
		{name: "whole line commented", line: "  # - job_name: 'pre-processor'", want: "  "},
		{name: "hash inside single quotes is data", line: "      - targets: ['probe:9115', 'probe # 2:9115']", want: "      - targets: ['probe:9115', 'probe # 2:9115']"},
		{name: "hash inside double quotes is data", line: `      - targets: ["probe # 2:9115"]`, want: `      - targets: ["probe # 2:9115"]`},
		{name: "hash not preceded by space is data", line: "    metrics_path: /metrics#frag", want: "    metrics_path: /metrics#frag"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stripYAMLComment(tt.line); got != tt.want {
				t.Errorf("stripYAMLComment(%q) = %q, want %q", tt.line, got, tt.want)
			}
		})
	}
}

// TestParseScrapeJobs pins the prometheus.yml parser the drift guard depends
// on. A job the parser cannot see is a job the guard cannot require, so a
// parsing hole silently reopens the very defect the guard exists to prevent.
func TestParseScrapeJobs(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want []scrapeJob
	}{
		{
			name: "single-quoted job with one target",
			yaml: "  - job_name: 'alt-backend'\n    static_configs:\n      - targets: ['alt-backend:9000']\n",
			want: []scrapeJob{{name: "alt-backend", targets: 1}},
		},
		{
			name: "double-quoted and unquoted forms",
			yaml: "  - job_name: \"mq-hub\"\n  - job_name: cadvisor\n",
			want: []scrapeJob{{name: "mq-hub"}, {name: "cadvisor"}},
		},
		{
			name: "trailing comment does not hide the job",
			yaml: "  - job_name: 'search-indexer'   # added for the new index tier\n    static_configs:\n      - targets: ['search-indexer:9300']\n",
			want: []scrapeJob{{name: "search-indexer", targets: 1}},
		},
		{
			name: "commented-out job is excluded",
			yaml: "  # - job_name: 'pre-processor'\n  #   static_configs:\n  #     - targets: ['pre-processor:9201']\n",
			want: nil,
		},
		{
			name: "multi-target job counts every host",
			yaml: "  - job_name: 'pki-agent'\n    static_configs:\n      - targets: ['pki-agent-alt-backend:9510']\n        labels: { subject: 'alt-backend' }\n      - targets: ['pki-agent-auth-hub:9510']\n        labels: { subject: 'auth-hub' }\n",
			want: []scrapeJob{{name: "pki-agent", targets: 2}},
		},
		{
			name: "several hosts on one targets line count separately",
			yaml: "  - job_name: 'pair'\n    static_configs:\n      - targets: ['a:1', 'b:2']\n",
			want: []scrapeJob{{name: "pair", targets: 2}},
		},
		{
			name: "inline comment on a targets line still counts the host",
			yaml: "  - job_name: 'plecto-proxy'\n    static_configs:\n      - targets: ['plecto-proxy:8080']  # admin port\n",
			want: []scrapeJob{{name: "plecto-proxy", targets: 1}},
		},
		{
			name: "non-job lines are ignored",
			yaml: "global:\n  scrape_interval: 15s\nscrape_configs:\n",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseScrapeJobs(strings.NewReader(tt.yaml))
			if err != nil {
				t.Fatalf("parseScrapeJobs: %v", err)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("parseScrapeJobs = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestAllowlist_AvailabilityJobsMatchPrometheusScrapeConfig pins the
// availability tile's job matcher to the jobs Prometheus actually scrapes.
// Drift is silent in production: a job in the matcher but not in prometheus.yml
// can never return a sample, and a scraped job missing from the matcher is
// omitted from the tile entirely — a fully-down edge proxy renders as no row
// at all, indistinguishable from a healthy stack.
func TestAllowlist_AvailabilityJobsMatchPrometheusScrapeConfig(t *testing.T) {
	scraped := prometheusScrapeJobs(t, findPrometheusConfig(t))
	_, allowed := availabilityJobMatcher(t)

	var omitted []string
	for _, job := range scraped {
		if !slices.Contains(allowed, job.name) {
			omitted = append(omitted, job.name)
		}
	}
	if len(omitted) > 0 {
		t.Errorf("availability matcher omits scraped jobs %v; an outage on them renders as no row at all", omitted)
	}

	var phantom []string
	for _, job := range allowed {
		if !slices.ContainsFunc(scraped, func(s scrapeJob) bool { return s.name == job }) {
			phantom = append(phantom, job)
		}
	}
	if len(phantom) > 0 {
		t.Errorf("availability matcher lists jobs %v that %s never scrapes", phantom, prometheusConfigRelPath)
	}
}

// TestAllowlist_AvailabilityAggregatesMultiTargetJobs guards the other way a
// down target can go unseen. Both consumers of `availability_services` key
// their rows by `job` alone (ServiceHealthTable's byJob map,
// ServiceREDTable's latestByJob record), so a job with N targets — pki-agent
// runs one sidecar per east-west service — collapses to whichever series
// arrived last. One dead sidecar out of eight then reads "up" seven times out
// of eight. Aggregating per job in PromQL makes the row the worst of its
// targets, which is what an availability tile must show.
func TestAllowlist_AvailabilityAggregatesMultiTargetJobs(t *testing.T) {
	scraped := prometheusScrapeJobs(t, findPrometheusConfig(t))
	entry, _ := availabilityJobMatcher(t)

	var multi []string
	targets := 0
	for _, job := range scraped {
		targets += job.targets
		if job.targets > 1 {
			multi = append(multi, job.name)
		}
	}
	// Without this the guard would pass vacuously the day the target parser
	// stops understanding prometheus.yml — the same silent-hole failure the
	// job-name parser already had.
	if targets == 0 {
		t.Fatalf("parsed %d jobs but zero targets from %s; the target parser is broken", len(scraped), prometheusConfigRelPath)
	}
	// Asserted unconditionally rather than only when a multi-target job
	// exists: `min by (job)` is a no-op for single-target jobs, and making the
	// requirement conditional would let a parser regression quietly license
	// removing the aggregation.
	if !strings.Contains(entry.promql, "min by (job)") {
		t.Errorf("availability promql does not aggregate per job: %s\n"+
			"multi-target jobs %v collapse to one row keyed by job, so without `min by (job)` a single down target is masked by its healthy siblings", entry.promql, multi)
	}
}
