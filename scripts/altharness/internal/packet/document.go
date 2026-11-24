package packet

import (
	"os"
	"strings"
)

// Document is a parsed TaskPacket or TaskHandoff.
type Document struct {
	Data       map[string]any
	IsMarkdown bool
	Body       []string
}

type fencedBlock struct {
	lang      string
	startLine int
	lines     []lineRec
}

type lineRec struct {
	no   int
	text string
}

func extractFrontmatter(lines []string) ([]string, []string, error) {
	if len(lines) == 0 || strings.TrimSpace(strings.TrimPrefix(lines[0], "\uFEFF")) != "---" {
		return nil, lines, nil
	}
	for i := 1; i < len(lines); i++ {
		s := strings.TrimSpace(lines[i])
		if s == "---" || s == "..." {
			return lines[1:i], lines[i+1:], nil
		}
	}
	return nil, nil, pErr("unterminated YAML frontmatter (missing closing '---')")
}

func splitMarkdownBlocks(lines []string) ([]lineRec, []fencedBlock) {
	var nonFenced []lineRec
	var blocks []fencedBlock
	inFence := false
	lang := ""
	start := 0
	var fenceLines []lineRec
	for i, line := range lines {
		lineNo := i + 1
		stripped := strings.TrimLeftFunc(line, unicodeSpace)
		if strings.HasPrefix(stripped, "```") {
			if !inFence {
				inFence = true
				info := strings.TrimSpace(stripped[3:])
				lang = ""
				if info != "" {
					lang = strings.Fields(info)[0]
				}
				start = lineNo
				fenceLines = nil
			} else {
				blocks = append(blocks, fencedBlock{lang: lang, startLine: start, lines: fenceLines})
				inFence = false
			}
			continue
		}
		if inFence {
			fenceLines = append(fenceLines, lineRec{no: lineNo, text: line})
		} else {
			nonFenced = append(nonFenced, lineRec{no: lineNo, text: line})
		}
	}
	return nonFenced, blocks
}

func unicodeSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\v' || r == '\f' || r == '\r'
}

func extractJSONBlock(blocks []fencedBlock) (map[string]any, error) {
	for _, block := range blocks {
		if strings.ToLower(block.lang) != "json" {
			continue
		}
		var b strings.Builder
		for i, line := range block.lines {
			if i > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(line.text)
		}
		parsed, err := parseJSONBytes([]byte(b.String()))
		if err != nil {
			return nil, pErr("invalid JSON code block at line %d: %s", block.startLine, err.Error())
		}
		m, ok := parsed.(map[string]any)
		if !ok {
			return nil, pErr("JSON code block at line %d must be an object", block.startLine)
		}
		return m, nil
	}
	return nil, nil
}

func intersectsStructured(data map[string]any) bool {
	for k := range data {
		if _, ok := structuredPayloadKeys[k]; ok {
			return true
		}
	}
	return false
}

func parseDocumentText(text string, jsonHint bool) (*Document, error) {
	stripped := strings.TrimLeft(strings.TrimPrefix(text, "\uFEFF"), " \t\r\n")
	if jsonHint || strings.HasPrefix(stripped, "{") {
		parsed, err := parseJSONBytes([]byte(strings.TrimPrefix(text, "\uFEFF")))
		if err != nil {
			return nil, pErr("invalid JSON document: %s", err.Error())
		}
		m, ok := parsed.(map[string]any)
		if !ok {
			return nil, pErr("JSON document root must be an object")
		}
		return &Document{Data: m, IsMarkdown: false}, nil
	}
	lines := splitLines(text)
	fmLines, body, err := extractFrontmatter(lines)
	if err != nil {
		return nil, err
	}
	if fmLines != nil {
		fm, err := parseYAMLSubset(fmLines)
		if err != nil {
			return nil, err
		}
		if intersectsStructured(fm) {
			return &Document{Data: fm, IsMarkdown: true, Body: body}, nil
		}
	}
	_, blocks := splitMarkdownBlocks(body)
	js, err := extractJSONBlock(blocks)
	if err != nil {
		return nil, err
	}
	if js != nil {
		return &Document{Data: js, IsMarkdown: true, Body: body}, nil
	}
	return nil, pErr("Markdown document must contain structured TaskPacket/TaskHandoff YAML frontmatter or a fenced ```json code block")
}

func loadDocument(path string) (*Document, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	jsonHint := strings.HasSuffix(strings.ToLower(path), ".json")
	return parseDocumentText(string(b), jsonHint)
}

func loadSchema(path string) (map[string]any, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	parsed, err := parseJSONBytes(b)
	if err != nil {
		return nil, pErr("schema root must be an object: %s", path)
	}
	m, ok := parsed.(map[string]any)
	if !ok {
		return nil, pErr("schema root must be an object: %s", path)
	}
	return m, nil
}
