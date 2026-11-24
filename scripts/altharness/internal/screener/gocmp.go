package screener

import (
	"fmt"
	"sort"
	"strings"
)

func compareGo(logical string, before, after FileSummary) []Candidate {
	var out []Candidate
	var removed []string
	for name := range before.Functions {
		if _, ok := after.Functions[name]; !ok {
			removed = append(removed, name)
		}
	}
	sort.Strings(removed)
	for _, name := range removed {
		out = append(out, makeCandidate(logical, name, "go.function_removed", "defined ("+name+")", "<removed>"))
	}
	var common []string
	for name := range before.Functions {
		if _, ok := after.Functions[name]; ok {
			common = append(common, name)
		}
	}
	sort.Strings(common)
	for _, name := range common {
		out = append(out, compareGoFunc(logical, name, before.Functions[name], after.Functions[name])...)
	}
	return out
}

func compareGoFunc(logical, name string, before, after *FuncSummary) []Candidate {
	if before == nil || after == nil {
		return nil
	}
	var out []Candidate
	if !sameCounts(before.CondOps, after.CondOps) {
		out = append(out, makeCandidate(logical, name, "go.condition_operator_change", fmtCounter(before.CondOps), fmtCounter(after.CondOps)))
	}
	if before.ReturnCount != after.ReturnCount || before.DeferCount != after.DeferCount {
		out = append(out, makeCandidate(
			logical, name, "go.return_or_defer_count_change",
			fmt.Sprintf("return=%d, defer=%d", before.ReturnCount, before.DeferCount),
			fmt.Sprintf("return=%d, defer=%d", after.ReturnCount, after.DeferCount),
		))
	}
	bSrv := canonicalHTTP(before.HTTPServerConfigs)
	aSrv := canonicalHTTP(after.HTTPServerConfigs)
	if !sameStringSlice(bSrv, aSrv) {
		out = append(out, makeCandidate(logical, name, "go.http_server_fields_change", joinOrNone(bSrv, "; "), joinOrNone(aSrv, "; ")))
	}
	bModes := canonicalModes(before.FileModes)
	aModes := canonicalModes(after.FileModes)
	if !sameStringSlice(bModes, aModes) {
		out = append(out, makeCandidate(logical, name, "go.file_mode_change", joinOrNone(bModes, ", "), joinOrNone(aModes, ", ")))
	}
	bLim := sortedStrings(before.BodyLimits)
	aLim := sortedStrings(after.BodyLimits)
	if !sameStringSlice(bLim, aLim) {
		out = append(out, makeCandidate(logical, name, "go.body_limit_change", joinOrNone(bLim, ", "), joinOrNone(aLim, ", ")))
	}
	bRP := sortedStrings(before.ReverseProxyModes)
	aRP := sortedStrings(after.ReverseProxyModes)
	if !sameStringSlice(bRP, aRP) {
		out = append(out, makeCandidate(logical, name, "go.reverse_proxy_rewrite_change", joinOrNone(bRP, ", "), joinOrNone(aRP, ", ")))
	}
	if !sameCounts(before.OutputEscapers, after.OutputEscapers) {
		out = append(out, makeCandidate(logical, name, "go.output_escaping_change", fmtCounter(before.OutputEscapers), fmtCounter(after.OutputEscapers)))
	}
	if after.SlogDiscardCount > before.SlogDiscardCount {
		out = append(out, makeCandidate(
			logical, name, "go.test_logger_discard_change",
			fmt.Sprintf("slog.DiscardHandler=%d, other_handlers=%d", before.SlogDiscardCount, before.SlogOtherHandlers),
			fmt.Sprintf("slog.DiscardHandler=%d, other_handlers=%d", after.SlogDiscardCount, after.SlogOtherHandlers),
		))
	}
	if before.ReceiverKind != after.ReceiverKind {
		bRecv, aRecv := before.ReceiverKind, after.ReceiverKind
		if bRecv == "" {
			bRecv = "none"
		}
		if aRecv == "" {
			aRecv = "none"
		}
		out = append(out, makeCandidate(logical, name, "go.receiver_pointer_change", bRecv, aRecv))
	}
	return out
}

func canonicalHTTP(configs []map[string]string) []string {
	out := make([]string, 0, len(configs))
	for _, cfg := range configs {
		keys := make([]string, 0, len(cfg))
		for k := range cfg {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, len(keys))
		for i, k := range keys {
			parts[i] = k + ":" + cfg[k]
		}
		out = append(out, "{"+strings.Join(parts, ", ")+"}")
	}
	sort.Strings(out)
	return out
}

func canonicalModes(items []CallArg) []string {
	out := make([]string, len(items))
	for i, item := range items {
		out[i] = item.Func + "(" + item.Value + ")"
	}
	sort.Strings(out)
	return out
}

func sameStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
