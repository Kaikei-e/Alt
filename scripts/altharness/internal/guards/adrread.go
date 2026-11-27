package guards

import (
	"alt/scripts/altharness/internal/fsperm"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	adrReadBlockedRC    = 2
	adrReadMaxRecursion = 32
)

type adrReadHit struct {
	Matched bool
	IDs     []string
}

type adrReadSeg struct {
	tokens         []Token
	precededByPipe bool
	substitutions  []string
}

var (
	adrNumberedFileRE = regexp.MustCompile(`(?:^|/)docs/ADR/(\d{6})\.md$`)
	adrDirRE          = regexp.MustCompile(`(?:^|/)docs/ADR/?$`)
	adrDirectChildRE  = regexp.MustCompile(`(?:^|/)docs/ADR/([^/]+)$`)
	adrReadSanitiseRE = regexp.MustCompile(`[^A-Za-z0-9_.-]`)
	redirStandaloneRE = regexp.MustCompile(`^(?:\d+|&)?(?:>>|>\||>&|>|<<-|<<|<<<|<&)$`)
	redirAttachedRE   = regexp.MustCompile(`^(?:\d+|&)?(?:>>|>\||>&|>|<<-|<<|<<<|<&).+$`)

	watchedAdrReaders = map[string]struct{}{
		"grep": {}, "egrep": {}, "fgrep": {}, "rg": {},
		"sed": {},
		"awk": {}, "gawk": {}, "mawk": {}, "nawk": {},
		"cat": {}, "head": {}, "tail": {}, "less": {},
	}

	grepValueFlags = map[string]struct{}{
		"-m": {}, "--max-count": {},
		"-A": {}, "--after-context": {},
		"-B": {}, "--before-context": {},
		"-C": {}, "--context": {},
		"-D": {}, "--devices": {},
		"-d": {}, "--directories": {},
		"--exclude": {}, "--exclude-from": {}, "--exclude-dir": {}, "--include": {},
		"--label": {}, "--binary-files": {}, "--group-separator": {},
	}

	grepPatternFlags = map[string]struct{}{
		"-e": {}, "--regexp": {}, "-f": {}, "--file": {},
	}

	rgValueFlags = map[string]struct{}{
		"-A": {}, "--after-context": {},
		"-B": {}, "--before-context": {},
		"-C": {}, "--context": {},
		"-m": {}, "--max-count": {},
		"-g": {}, "--glob": {}, "--iglob": {},
		"-t": {}, "--type": {}, "-T": {}, "--type-not": {},
		"--type-add": {}, "--type-clear": {},
		"-d": {}, "--max-depth": {}, "--maxdepth": {},
		"-M": {}, "--max-columns": {}, "--max-filesize": {},
		"-j": {}, "--threads": {},
		"-E": {}, "--encoding": {},
		"-r": {}, "--replace": {},
		"--color": {}, "--colors": {}, "--engine": {},
		"--field-context-separator": {}, "--field-match-separator": {},
		"--context-separator": {}, "--path-separator": {},
		"--pre": {}, "--pre-glob": {},
		"--sort": {}, "--sortr": {}, "--ignore-file": {},
		"--hostname-bin": {}, "--hyperlink-format": {},
		"--dfa-size-limit": {}, "--regex-size-limit": {},
	}

	sedValueFlags = map[string]struct{}{
		"-l": {}, "--line-length": {},
	}

	sedScriptFlags = map[string]struct{}{
		"-e": {}, "--expression": {}, "-f": {}, "--file": {},
	}

	awkValueFlags = map[string]struct{}{
		"-F": {}, "--field-separator": {},
		"-v": {}, "--assign": {},
		"-E": {}, "--exec": {},
		"-i": {}, "--include": {},
		"-l": {}, "--load": {},
		"-W": {},
	}

	awkProgramFlags = map[string]struct{}{
		"-f": {}, "--file": {}, "-e": {}, "--source": {},
	}

	headTailValueFlags = map[string]struct{}{
		"-n": {}, "--lines": {},
		"-c": {}, "--bytes": {},
		"-s": {}, "--sleep-interval": {},
		"--pid": {}, "--max-unchanged-stats": {},
	}

	lessValueFlags = map[string]struct{}{
		"-b": {}, "--buffers": {},
		"-h": {}, "--max-back-scroll": {},
		"-j": {}, "--jump-target": {},
		"-k": {}, "--lesskey-file": {},
		"-o": {}, "--log-file": {},
		"-O": {}, "--LOG-FILE": {},
		"-p": {}, "--pattern": {},
		"-P": {}, "--prompt": {},
		"-t": {}, "--tag": {},
		"-T": {}, "--tags-file": {},
		"-x": {}, "--tabs": {},
		"-y": {}, "--max-forw-scroll": {},
		"-z": {}, "--window": {},
		"-#": {}, "--shift": {},
	}
)

// RunAdrRead is the Claude/Codex PreToolUse Bash guard for direct shell ADR reads.
// First match per session writes a receipt and exits 2 with docdag guidance; subsequent
// matches in the same session exit 0. Broken payloads / receipt IO errors fail closed (exit 2).
func RunAdrRead(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	_ = ctx
	_ = args
	_ = stdout
	if stderr == nil {
		stderr = io.Discard
	}
	if os.Getenv("ALT_ADR_PREFLIGHT") == "disabled" {
		return 0
	}
	if stdin == nil {
		fmt.Fprintln(stderr, "BLOCKED: adr-read guard could not read hook payload (missing stdin).")
		return adrReadBlockedRC
	}
	raw, err := io.ReadAll(stdin)
	if err != nil {
		fmt.Fprintf(stderr, "BLOCKED: adr-read guard could not read hook payload (%v).\n", err)
		return adrReadBlockedRC
	}
	if !validUTF8(string(raw)) {
		fmt.Fprintln(stderr, "BLOCKED: adr-read guard could not read hook payload (invalid UTF-8 stdin).")
		return adrReadBlockedRC
	}

	cmdStr, cmdList, cwd, sessionID, agentID, ignore, perr := parseAdrReadHookPayload(raw)
	if perr != nil {
		fmt.Fprintf(stderr, "BLOCKED: adr-read guard could not read hook payload (%v).\n", perr)
		return adrReadBlockedRC
	}
	if ignore {
		return 0
	}

	held, reason, err := evaluateAdrRead(cmdStr, cmdList, cwd, sessionID, agentID)
	if err != nil {
		fmt.Fprintf(stderr, "BLOCKED: adr-read guard failed (%v).\n", err)
		return adrReadBlockedRC
	}
	if held {
		fmt.Fprintln(stderr, reason)
		return adrReadBlockedRC
	}
	return 0
}

func parseAdrReadHookPayload(raw []byte) (cmdStr string, cmdList []string, cwd, sessionID, agentID string, ignore bool, err error) {
	var payload map[string]any
	if uerr := json.Unmarshal(raw, &payload); uerr != nil {
		return "", nil, "", "", "", false, fmt.Errorf("invalid JSON (%s)", jsonErrMsg(uerr))
	}
	if payload == nil {
		return "", nil, "", "", "", false, fmt.Errorf("expected JSON object")
	}
	tool, _ := payload["tool_name"].(string)
	tool = strings.TrimSpace(tool)
	if tool == "" {
		return "", nil, "", "", "", false, fmt.Errorf("missing tool_name")
	}
	if tool != "Bash" && tool != "shell" && tool != "run_command" {
		return "", nil, "", "", "", true, nil
	}
	input, ok := payload["tool_input"].(map[string]any)
	if !ok {
		return "", nil, "", "", "", false, fmt.Errorf("missing tool_input for %s", tool)
	}
	cStr, cList, cOK := extractAgyCommand(input)
	if !cOK {
		return "", nil, "", "", "", false, fmt.Errorf("missing or invalid command for %s", tool)
	}
	cwd = agyCwd(payload)
	sessionID = agySessionID(payload)
	agentID, _ = payload["agent_id"].(string)
	return cStr, cList, cwd, sessionID, agentID, false, nil
}

// evaluateAdrRead runs ADR-read detection + first-hold session receipt handling.
// Shared by both `alt-harness guard-adr-read` and `alt-harness agy-hook pre-adr-read`.
func evaluateAdrRead(cmdStr string, cmdList []string, cwd, sessionID, agentID string) (held bool, reason string, err error) {
	if os.Getenv("ALT_ADR_PREFLIGHT") == "disabled" {
		return false, "", nil
	}

	var hit adrReadHit
	if cmdList != nil {
		hit, err = detectAdrReadArgv(cmdList, cwd, 0)
	} else {
		hit, err = detectAdrReadCommand(cmdStr, cwd, 0)
	}
	if err != nil {
		return false, "", err
	}
	if !hit.Matched {
		return false, "", nil
	}

	receiptPath := adrReadReceiptPath(sessionID, agentID)
	if _, serr := os.Stat(receiptPath); serr == nil {
		return false, "", nil
	} else if !errors.Is(serr, os.ErrNotExist) {
		return false, "", fmt.Errorf("cannot inspect receipt %s: %w", receiptPath, serr)
	}

	if mkErr := os.MkdirAll(filepath.Dir(receiptPath), fsperm.Dir); mkErr != nil {
		return false, "", fmt.Errorf("cannot create receipt directory: %w", mkErr)
	}
	if wErr := os.WriteFile(receiptPath, []byte("held\n"), fsperm.File); wErr != nil {
		return false, "", fmt.Errorf("cannot write receipt %s: %w", receiptPath, wErr)
	}

	return true, formatAdrReadGuidance(hit), nil
}

func adrReadSanitiseToken(raw, fallback string) string {
	cleaned := adrReadSanitiseRE.ReplaceAllString(raw, "_")
	if cleaned == "" || cleaned == "." || cleaned == ".." {
		return fallback
	}
	return cleaned
}

func adrReadStateDir() string {
	if override := os.Getenv("ALT_ADR_PREFLIGHT_STATE_DIR"); override != "" {
		return override
	}
	if xdg := os.Getenv("XDG_STATE_HOME"); xdg != "" {
		return filepath.Join(xdg, "alt-adr-preflight")
	}
	home := os.Getenv("HOME")
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	return filepath.Join(home, ".local", "state", "alt-adr-preflight")
}

func adrReadReceiptPath(sessionID, agentID string) string {
	sessTok := adrReadSanitiseToken(sessionID, "default")
	agentRaw := agentID
	if agentRaw == "" {
		agentRaw = "main"
	}
	agentTok := adrReadSanitiseToken(agentRaw, "main")
	return filepath.Join(adrReadStateDir(), sessTok, agentTok, "adr-read.receipt")
}

func formatAdrReadGuidance(hit adrReadHit) string {
	if len(hit.IDs) > 0 {
		primary := hit.IDs[0]
		return strings.Join([]string{
			fmt.Sprintf("ADR READ GUARD (one-time per session): prefer `docdag` over direct shell reads of `docs/ADR/` (detected: %s).", strings.Join(hit.IDs, ", ")),
			"Try instead:",
			fmt.Sprintf("  docdag context %s", primary),
			fmt.Sprintf("  docdag context %s --section Consequences", primary),
			fmt.Sprintf("  docdag resolve %s --fields id,title,status", primary),
			"  docdag query --binding --where affected_services=contains:<svc> --fields id,title,status",
			"This hold happens only once per session; re-run the same command to proceed if raw text outside docdag is needed.",
		}, "\n")
	}
	return strings.Join([]string{
		"ADR READ GUARD (one-time per session): prefer `docdag` over direct shell search of `docs/ADR/`.",
		"Try instead:",
		"  docdag query --binding --fields id,title,status",
		"  docdag query --binding --where affected_services=contains:<svc> --fields id,title,status",
		"  docdag context <id>",
		"  docdag context <id> --section Consequences",
		"  docdag resolve <id> --fields id,title,status",
		"This hold happens only once per session; re-run the same command to proceed if raw text outside docdag is needed.",
	}, "\n")
}

func splitAdrReadSegments(command string) []adrReadSeg {
	state := &splitState{command: command, length: len(command), quote: "NORMAL"}
	var segs []adrReadSeg
	var curSubs []string
	nextPipe := false

	flush := func(upcomingPipe bool) {
		state.flushToken()
		if len(state.currentSeg) > 0 || len(curSubs) > 0 {
			segs = append(segs, adrReadSeg{
				tokens:         append([]Token(nil), state.currentSeg...),
				precededByPipe: nextPipe,
				substitutions:  append([]string(nil), curSubs...),
			})
			state.currentSeg = state.currentSeg[:0]
			curSubs = curSubs[:0]
		}
		nextPipe = upcomingPipe
	}

	i := 0
	for i < state.length {
		switch state.quote {
		case "SINGLE":
			i = scanSingle(state, i)
		case "ANSI_SINGLE":
			i = scanAnsiSingle(state, i)
		case "DOUBLE":
			ch := state.command[i]
			if ch == '$' && i+1 < state.length && state.command[i+1] == '(' {
				inner, next := scanDollarParen(state.command, i+2, state.length)
				curSubs = append(curSubs, inner)
				state.buf = append(state.buf, state.command[i:min(next, state.length)]...)
				i = next
				continue
			}
			if ch == '`' {
				inner, next := scanBacktick(state.command, i+1, state.length)
				curSubs = append(curSubs, inner)
				state.buf = append(state.buf, state.command[i:min(next, state.length)]...)
				i = next
				continue
			}
			i = scanDouble(state, i)
		default:
			ch := state.command[i]
			if ch == '$' && i+1 < state.length && state.command[i+1] == '(' {
				inner, next := scanDollarParen(state.command, i+2, state.length)
				curSubs = append(curSubs, inner)
				state.hasToken = true
				state.buf = append(state.buf, state.command[i:min(next, state.length)]...)
				i = next
				continue
			}
			if ch == '`' {
				inner, next := scanBacktick(state.command, i+1, state.length)
				curSubs = append(curSubs, inner)
				state.hasToken = true
				state.buf = append(state.buf, state.command[i:min(next, state.length)]...)
				i = next
				continue
			}
			if ch == '\n' || ch == ';' || ch == '(' || ch == ')' || ch == '&' || ch == '|' {
				upcomingPipe := false
				step := 1
				if ch == '|' {
					if i+1 < state.length && state.command[i+1] == '|' {
						step = 2
					} else if i+1 < state.length && state.command[i+1] == '&' {
						step = 2
						upcomingPipe = true
					} else {
						upcomingPipe = true
					}
				} else if ch == '&' && i+1 < state.length && state.command[i+1] == '&' {
					step = 2
				}
				flush(upcomingPipe)
				i += step
				continue
			}
			i = scanNormal(state, i)
		}
	}
	flush(false)
	return segs
}

func mergeAdrHit(dst *adrReadHit, src adrReadHit) {
	if !src.Matched {
		return
	}
	dst.Matched = true
	if len(src.IDs) == 0 {
		return
	}
	seen := make(map[string]struct{}, len(dst.IDs)+len(src.IDs))
	for _, id := range dst.IDs {
		seen[id] = struct{}{}
	}
	for _, id := range src.IDs {
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			dst.IDs = append(dst.IDs, id)
		}
	}
	sort.Strings(dst.IDs)
}

func normalizeCwd(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	return path.Clean(strings.ReplaceAll(s, `\`, "/"))
}

func applyVirtualDirChange(cmdBase string, args []string, virtualCwd string, dirStack *[]string) (string, bool) {
	if cmdBase != "cd" && cmdBase != "pushd" && cmdBase != "popd" {
		return virtualCwd, false
	}
	if cmdBase == "popd" {
		if len(*dirStack) > 0 {
			top := (*dirStack)[len(*dirStack)-1]
			*dirStack = (*dirStack)[:len(*dirStack)-1]
			return top, true
		}
		return "", true
	}

	noSwitch := false
	i := 0
	for i < len(args) {
		tok := args[i]
		if tok == "--" {
			i++
			break
		}
		if strings.HasPrefix(tok, "-") && len(tok) > 1 {
			if cmdBase == "pushd" && strings.ContainsRune(tok[1:], 'n') {
				noSwitch = true
			}
			i++
			continue
		}
		break
	}
	if noSwitch {
		return virtualCwd, true
	}
	if i >= len(args) {
		return "", true
	}
	target := strings.ReplaceAll(strings.TrimSpace(args[i]), `\`, "/")
	if target == "" || target == "-" || target == "~" || strings.HasPrefix(target, "~/") || strings.Contains(target, "$") {
		return "", true
	}
	if cmdBase == "pushd" {
		*dirStack = append(*dirStack, virtualCwd)
	}
	if strings.HasPrefix(target, "/") {
		return path.Clean(target), true
	}
	if virtualCwd != "" {
		return path.Clean(virtualCwd + "/" + target), true
	}
	return path.Clean(target), true
}

func detectAdrReadCommand(command, initialCwd string, depth int) (adrReadHit, error) {
	if depth > adrReadMaxRecursion {
		return adrReadHit{}, fmt.Errorf("command nesting exceeded maximum recursion depth")
	}
	if strings.TrimSpace(command) == "" {
		return adrReadHit{}, nil
	}

	virtualCwd := normalizeCwd(initialCwd)
	var dirStack []string
	var result adrReadHit

	for _, seg := range splitAdrReadSegments(command) {
		for _, sub := range seg.substitutions {
			subHit, err := detectAdrReadCommand(sub, virtualCwd, depth+1)
			if err != nil {
				return adrReadHit{}, err
			}
			mergeAdrHit(&result, subHit)
		}

		segHit, nextCwd, err := inspectAdrReadSegment(seg.tokens, seg.precededByPipe, true, virtualCwd, &dirStack, depth)
		if err != nil {
			return adrReadHit{}, err
		}
		virtualCwd = nextCwd
		mergeAdrHit(&result, segHit)
	}
	return result, nil
}

func detectAdrReadArgv(argv []string, initialCwd string, depth int) (adrReadHit, error) {
	if len(argv) == 0 {
		return adrReadHit{}, nil
	}
	tokens := make([]Token, len(argv))
	for i, part := range argv {
		tokens[i] = Token{Text: part, Quoted: false}
	}
	var dirStack []string
	hit, _, err := inspectAdrReadSegment(tokens, false, false, normalizeCwd(initialCwd), &dirStack, depth)
	return hit, err
}

func extractRedirections(tokens []Token) ([]Token, []string) {
	clean := make([]Token, 0, len(tokens))
	var inputTargets []string

	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		if tok.Quoted {
			clean = append(clean, tok)
			continue
		}
		text := tok.Text
		if text == "<" || text == "0<" || text == "<>" {
			if i+1 < len(tokens) {
				inputTargets = append(inputTargets, tokens[i+1].Text)
				i++
			}
			continue
		}
		if strings.HasPrefix(text, "0<") && !strings.HasPrefix(text, "0<<") && !strings.HasPrefix(text, "0<&") {
			if rest := text[2:]; rest != "" {
				inputTargets = append(inputTargets, rest)
			}
			continue
		}
		if strings.HasPrefix(text, "<>") && len(text) > 2 {
			inputTargets = append(inputTargets, text[2:])
			continue
		}
		if strings.HasPrefix(text, "<") && !strings.HasPrefix(text, "<<") && !strings.HasPrefix(text, "<&") && !strings.HasPrefix(text, "<(") {
			if rest := text[1:]; rest != "" {
				inputTargets = append(inputTargets, rest)
			}
			continue
		}
		if redirStandaloneRE.MatchString(text) {
			i++
			continue
		}
		if redirAttachedRE.MatchString(text) {
			continue
		}
		clean = append(clean, tok)
	}
	return clean, inputTargets
}

func inspectAdrReadSegment(
	tokens []Token,
	precededByPipe bool,
	parseRedirs bool,
	virtualCwd string,
	dirStack *[]string,
	depth int,
) (adrReadHit, string, error) {
	var result adrReadHit
	if len(tokens) == 0 {
		return result, virtualCwd, nil
	}

	cleanTokens := tokens
	var inputRedirects []string
	if parseRedirs {
		cleanTokens, inputRedirects = extractRedirections(tokens)
	}

	remaining, _, _, extra := unwrapCommandPrefix(cleanTokens)
	for _, script := range extra {
		subHit, err := detectAdrReadCommand(script, virtualCwd, depth+1)
		if err != nil {
			return adrReadHit{}, virtualCwd, err
		}
		mergeAdrHit(&result, subHit)
	}
	if len(remaining) == 0 {
		return result, virtualCwd, nil
	}

	cmdBase := baseName(remaining[0].Text)
	args := make([]string, len(remaining)-1)
	for i, t := range remaining[1:] {
		args[i] = t.Text
	}

	if nextCwd, changed := applyVirtualDirChange(cmdBase, args, virtualCwd, dirStack); changed {
		return result, nextCwd, nil
	}

	if _, isReader := watchedAdrReaders[cmdBase]; isReader {
		targets := extractReaderTargets(cmdBase, args, inputRedirects, precededByPipe)
		for _, rawTarget := range targets {
			if matched, id := classifyADRTarget(rawTarget, virtualCwd); matched {
				var ids []string
				if id != "" {
					ids = []string{id}
				}
				mergeAdrHit(&result, adrReadHit{Matched: true, IDs: ids})
			}
		}
		return result, virtualCwd, nil
	}

	for _, nested := range extractNestedScripts(remaining) {
		subHit, err := detectAdrReadCommand(nested, virtualCwd, depth+1)
		if err != nil {
			return adrReadHit{}, virtualCwd, err
		}
		mergeAdrHit(&result, subHit)
	}
	return result, virtualCwd, nil
}

func extractReaderTargets(cmdBase string, args, inputRedirects []string, precededByPipe bool) []string {
	var targets []string
	switch cmdBase {
	case "grep", "egrep", "fgrep":
		targets = extractGrepTargets(args, false, precededByPipe, len(inputRedirects) > 0)
	case "rg":
		targets = extractGrepTargets(args, true, precededByPipe, len(inputRedirects) > 0)
	case "sed":
		targets = extractSedTargets(args)
	case "awk", "gawk", "mawk", "nawk":
		targets = extractAwkTargets(args)
	case "cat", "head", "tail", "less":
		targets = extractFileReaderTargets(cmdBase, args)
	}
	targets = append(targets, inputRedirects...)
	return targets
}

func extractGrepTargets(args []string, isRG, precededByPipe, hasInputRedir bool) []string {
	valueFlags := grepValueFlags
	shortValueChars := "mABCDd"
	if isRG {
		valueFlags = rgValueFlags
		shortValueChars = "ABCmgTtDdMjEr"
	}

	hasExplicitPattern := false
	isRecursive := isRG
	var positionals []string

	i := 0
	for i < len(args) {
		tok := args[i]
		if tok == "--" {
			positionals = append(positionals, args[i+1:]...)
			break
		}
		if tok == "-" {
			positionals = append(positionals, tok)
			i++
			continue
		}
		if isRG && tok == "--files" {
			hasExplicitPattern = true
			i++
			continue
		}
		if _, ok := grepPatternFlags[tok]; ok {
			hasExplicitPattern = true
			i += 2
			continue
		}
		if strings.HasPrefix(tok, "--regexp=") || strings.HasPrefix(tok, "--file=") {
			hasExplicitPattern = true
			i++
			continue
		}
		if _, ok := valueFlags[tok]; ok {
			i += 2
			continue
		}
		if strings.HasPrefix(tok, "--") {
			if !isRG && (tok == "--recursive" || tok == "--dereference-recursive") {
				isRecursive = true
			}
			i++
			continue
		}
		if strings.HasPrefix(tok, "-") && len(tok) > 1 {
			consumedNext := false
			for k := 1; k < len(tok); k++ {
				ch := tok[k]
				if ch == 'e' || ch == 'f' {
					hasExplicitPattern = true
					if k == len(tok)-1 {
						consumedNext = true
					}
					break
				}
				if !isRG && (ch == 'r' || ch == 'R') {
					isRecursive = true
				}
				if strings.IndexByte(shortValueChars, ch) >= 0 {
					if k == len(tok)-1 {
						consumedNext = true
					}
					break
				}
			}
			if consumedNext {
				i += 2
			} else {
				i++
			}
			continue
		}
		positionals = append(positionals, tok)
		i++
	}

	fileArgs := positionals
	if !hasExplicitPattern && len(fileArgs) > 0 {
		fileArgs = fileArgs[1:]
	}
	if len(fileArgs) == 0 && isRecursive && !precededByPipe && !hasInputRedir {
		return []string{"."}
	}
	return fileArgs
}

func extractSedTargets(args []string) []string {
	hasExplicitScript := false
	var positionals []string

	i := 0
	for i < len(args) {
		tok := args[i]
		if tok == "--" {
			positionals = append(positionals, args[i+1:]...)
			break
		}
		if tok == "-" {
			positionals = append(positionals, tok)
			i++
			continue
		}
		if _, ok := sedScriptFlags[tok]; ok {
			hasExplicitScript = true
			i += 2
			continue
		}
		if strings.HasPrefix(tok, "--expression=") || strings.HasPrefix(tok, "--file=") {
			hasExplicitScript = true
			i++
			continue
		}
		if _, ok := sedValueFlags[tok]; ok {
			i += 2
			continue
		}
		if strings.HasPrefix(tok, "--") {
			i++
			continue
		}
		if strings.HasPrefix(tok, "-") && len(tok) > 1 {
			consumedNext := false
			for k := 1; k < len(tok); k++ {
				ch := tok[k]
				if ch == 'e' || ch == 'f' {
					hasExplicitScript = true
					if k == len(tok)-1 {
						consumedNext = true
					}
					break
				}
				if ch == 'l' {
					if k == len(tok)-1 {
						consumedNext = true
					}
					break
				}
			}
			if consumedNext {
				i += 2
			} else {
				i++
			}
			continue
		}
		positionals = append(positionals, tok)
		i++
	}

	if !hasExplicitScript && len(positionals) > 0 {
		return positionals[1:]
	}
	return positionals
}

func extractAwkTargets(args []string) []string {
	hasExplicitProgram := false
	var positionals []string

	i := 0
	for i < len(args) {
		tok := args[i]
		if tok == "--" {
			positionals = append(positionals, args[i+1:]...)
			break
		}
		if tok == "-" {
			positionals = append(positionals, tok)
			i++
			continue
		}
		if _, ok := awkProgramFlags[tok]; ok {
			hasExplicitProgram = true
			i += 2
			continue
		}
		if strings.HasPrefix(tok, "--file=") || strings.HasPrefix(tok, "--source=") {
			hasExplicitProgram = true
			i++
			continue
		}
		if _, ok := awkValueFlags[tok]; ok {
			i += 2
			continue
		}
		if strings.HasPrefix(tok, "--") {
			i++
			continue
		}
		if strings.HasPrefix(tok, "-") && len(tok) > 1 {
			consumedNext := false
			for k := 1; k < len(tok); k++ {
				ch := tok[k]
				if ch == 'f' || ch == 'e' {
					hasExplicitProgram = true
					if k == len(tok)-1 {
						consumedNext = true
					}
					break
				}
				if ch == 'F' || ch == 'v' || ch == 'E' || ch == 'i' || ch == 'l' || ch == 'W' {
					if k == len(tok)-1 {
						consumedNext = true
					}
					break
				}
			}
			if consumedNext {
				i += 2
			} else {
				i++
			}
			continue
		}
		positionals = append(positionals, tok)
		i++
	}

	fileArgs := positionals
	if !hasExplicitProgram && len(fileArgs) > 0 {
		fileArgs = fileArgs[1:]
	}
	var files []string
	for _, arg := range fileArgs {
		if envAssignRE.MatchString(arg) {
			continue
		}
		files = append(files, arg)
	}
	return files
}

func extractFileReaderTargets(cmdBase string, args []string) []string {
	var valueFlags map[string]struct{}
	switch cmdBase {
	case "head", "tail":
		valueFlags = headTailValueFlags
	case "less":
		valueFlags = lessValueFlags
	}

	var files []string
	i := 0
	for i < len(args) {
		tok := args[i]
		if tok == "--" {
			files = append(files, args[i+1:]...)
			break
		}
		if tok == "-" {
			i++
			continue
		}
		if cmdBase == "less" && strings.HasPrefix(tok, "+") {
			i++
			continue
		}
		if valueFlags != nil {
			if _, ok := valueFlags[tok]; ok {
				i += 2
				continue
			}
		}
		if strings.HasPrefix(tok, "-") {
			i++
			continue
		}
		files = append(files, tok)
		i++
	}
	return files
}

func classifyADRTarget(rawTarget, virtualCwd string) (matched bool, adrID string) {
	t := strings.TrimSpace(rawTarget)
	if t == "" || t == "-" {
		return false, ""
	}
	t = strings.ReplaceAll(t, `\`, "/")

	var resolved string
	if strings.HasPrefix(t, "/") {
		resolved = path.Clean(t)
	} else if virtualCwd != "" {
		resolved = path.Clean(virtualCwd + "/" + t)
	} else {
		resolved = path.Clean(t)
	}

	if m := adrNumberedFileRE.FindStringSubmatch(resolved); len(m) == 2 {
		return true, m[1]
	}
	if adrDirRE.MatchString(resolved) {
		return true, ""
	}
	if m := adrDirectChildRE.FindStringSubmatch(resolved); len(m) == 2 {
		if strings.ContainsAny(m[1], "*?[") {
			return true, ""
		}
	}
	return false, ""
}
