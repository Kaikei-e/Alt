package ratchet

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

type jkind int

const (
	jNull jkind = iota
	jBool
	jInt
	jFloat
	jStr
	jArr
	jObj
)

type jpair struct {
	k string
	v jval
}

type jval struct {
	kind jkind
	b    bool
	i    string // canonical integer text
	f    float64
	s    string
	arr  []jval
	obj  []jpair
}

func (v jval) truthy() bool {
	switch v.kind {
	case jNull:
		return false
	case jBool:
		return v.b
	case jInt:
		return v.i != "0" && v.i != "-0"
	case jFloat:
		return v.f != 0
	case jStr:
		return v.s != ""
	case jArr:
		return len(v.arr) != 0
	case jObj:
		return len(v.obj) != 0
	default:
		return false
	}
}

func (v jval) asInt() (int64, bool) {
	switch v.kind {
	case jBool:
		if v.b {
			return 1, true
		}
		return 0, true
	case jInt:
		n, err := strconv.ParseInt(v.i, 10, 64)
		if err != nil {
			return 0, false
		}
		return n, true
	default:
		return 0, false
	}
}

func (v jval) asString() (string, bool) {
	if v.kind == jStr {
		return v.s, true
	}
	return "", false
}

func (v jval) repr() string {
	switch v.kind {
	case jNull:
		return "None"
	case jBool:
		if v.b {
			return "True"
		}
		return "False"
	case jInt:
		if v.i == "-0" {
			return "0"
		}
		return v.i
	case jFloat:
		return pyFloatRepr(v.f)
	case jStr:
		return pyReprString(v.s)
	case jArr:
		parts := make([]string, len(v.arr))
		for i, el := range v.arr {
			parts[i] = el.repr()
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case jObj:
		parts := make([]string, len(v.obj))
		for i, p := range v.obj {
			parts[i] = pyReprString(p.k) + ": " + p.v.repr()
		}
		return "{" + strings.Join(parts, ", ") + "}"
	default:
		return "None"
	}
}

func (v jval) field(key string) (jval, bool) {
	if v.kind != jObj {
		return jval{}, false
	}
	for _, p := range v.obj {
		if p.k == key {
			return p.v, true
		}
	}
	return jval{}, false
}

func (v jval) has(key string) bool {
	_, ok := v.field(key)
	return ok
}

type jsonError struct{ msg string }

func (e jsonError) Error() string { return e.msg }

func parseJSON(s string) (jval, error) {
	i := skipWS(s, 0)
	v, i, err := scanValue(s, i)
	if err != nil {
		return jval{}, err
	}
	i = skipWS(s, i)
	if i != len(s) {
		return jval{}, jsonErr("Extra data", s, i)
	}
	return v, nil
}

func skipWS(s string, i int) int {
	for i < len(s) {
		switch s[i] {
		case ' ', '\t', '\n', '\r':
			i++
		default:
			return i
		}
	}
	return i
}

func jsonErr(msg, s string, pos int) error {
	if pos < 0 {
		pos = 0
	}
	if pos > len(s) {
		pos = len(s)
	}
	line, col := 1, 1
	for i := 0; i < pos; i++ {
		if s[i] == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	return jsonError{fmt.Sprintf("%s: line %d column %d (char %d)", msg, line, col, pos)}
}

func scanValue(s string, i int) (jval, int, error) {
	if i >= len(s) {
		return jval{}, i, jsonErr("Expecting value", s, i)
	}
	switch s[i] {
	case '"':
		str, ni, err := scanJSONString(s, i)
		if err != nil {
			return jval{}, 0, err
		}
		return jval{kind: jStr, s: str}, ni, nil
	case '{':
		return scanObject(s, i+1)
	case '[':
		return scanArray(s, i+1)
	case 't':
		if strings.HasPrefix(s[i:], "true") {
			return jval{kind: jBool, b: true}, i + 4, nil
		}
		return jval{}, i, jsonErr("Expecting value", s, i)
	case 'f':
		if strings.HasPrefix(s[i:], "false") {
			return jval{kind: jBool, b: false}, i + 5, nil
		}
		return jval{}, i, jsonErr("Expecting value", s, i)
	case 'n':
		if strings.HasPrefix(s[i:], "null") {
			return jval{kind: jNull}, i + 4, nil
		}
		return jval{}, i, jsonErr("Expecting value", s, i)
	case 'N':
		if strings.HasPrefix(s[i:], "NaN") {
			return jval{kind: jFloat, f: mathNaN()}, i + 3, nil
		}
		return jval{}, i, jsonErr("Expecting value", s, i)
	case 'I':
		if strings.HasPrefix(s[i:], "Infinity") {
			return jval{kind: jFloat, f: mathInf(1)}, i + 8, nil
		}
		return jval{}, i, jsonErr("Expecting value", s, i)
	case '-':
		if strings.HasPrefix(s[i:], "-Infinity") {
			return jval{kind: jFloat, f: mathInf(-1)}, i + 9, nil
		}
		if v, ni, ok := scanNumber(s, i); ok {
			return v, ni, nil
		}
		return jval{}, i, jsonErr("Expecting value", s, i)
	default:
		if s[i] >= '0' && s[i] <= '9' {
			if v, ni, ok := scanNumber(s, i); ok {
				return v, ni, nil
			}
		}
		return jval{}, i, jsonErr("Expecting value", s, i)
	}
}

func scanObject(s string, i int) (jval, int, error) {
	i = skipWS(s, i)
	if i >= len(s) {
		return jval{}, 0, jsonErr("Expecting property name enclosed in double quotes", s, i)
	}
	if s[i] == '}' {
		return jval{kind: jObj}, i + 1, nil
	}
	var obj []jpair
	for {
		if i >= len(s) || s[i] != '"' {
			return jval{}, 0, jsonErr("Expecting property name enclosed in double quotes", s, i)
		}
		key, ni, err := scanJSONString(s, i)
		if err != nil {
			return jval{}, 0, err
		}
		i = ni
		i = skipWS(s, i)
		if i >= len(s) || s[i] != ':' {
			return jval{}, 0, jsonErr("Expecting ':' delimiter", s, i)
		}
		i++
		i = skipWS(s, i)
		val, ni, err := scanValue(s, i)
		if err != nil {
			return jval{}, 0, err
		}
		i = ni
		setPair(&obj, key, val)
		i = skipWS(s, i)
		if i >= len(s) {
			return jval{}, 0, jsonErr("Expecting ',' delimiter", s, i)
		}
		if s[i] == '}' {
			return jval{kind: jObj, obj: obj}, i + 1, nil
		}
		if s[i] != ',' {
			return jval{}, 0, jsonErr("Expecting ',' delimiter", s, i)
		}
		i++
		i = skipWS(s, i)
	}
}

func scanArray(s string, i int) (jval, int, error) {
	i = skipWS(s, i)
	if i < len(s) && s[i] == ']' {
		return jval{kind: jArr}, i + 1, nil
	}
	var arr []jval
	for {
		val, ni, err := scanValue(s, i)
		if err != nil {
			return jval{}, 0, err
		}
		i = ni
		arr = append(arr, val)
		i = skipWS(s, i)
		if i >= len(s) {
			return jval{}, 0, jsonErr("Expecting ',' delimiter", s, i)
		}
		if s[i] == ']' {
			return jval{kind: jArr, arr: arr}, i + 1, nil
		}
		if s[i] != ',' {
			return jval{}, 0, jsonErr("Expecting ',' delimiter", s, i)
		}
		i++
		i = skipWS(s, i)
	}
}

func setPair(obj *[]jpair, key string, val jval) {
	for i := range *obj {
		if (*obj)[i].k == key {
			(*obj)[i].v = val
			return
		}
	}
	*obj = append(*obj, jpair{k: key, v: val})
}

func scanNumber(s string, i int) (jval, int, bool) {
	start := i
	if i < len(s) && s[i] == '-' {
		i++
	}
	if i >= len(s) || s[i] < '0' || s[i] > '9' {
		return jval{}, start, false
	}
	if s[i] == '0' {
		i++
	} else {
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
	}
	isFloat := false
	if i < len(s) && s[i] == '.' {
		if i+1 < len(s) && s[i+1] >= '0' && s[i+1] <= '9' {
			isFloat = true
			i++
			for i < len(s) && s[i] >= '0' && s[i] <= '9' {
				i++
			}
		}
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		j := i + 1
		if j < len(s) && (s[j] == '+' || s[j] == '-') {
			j++
		}
		if j < len(s) && s[j] >= '0' && s[j] <= '9' {
			isFloat = true
			j++
			for j < len(s) && s[j] >= '0' && s[j] <= '9' {
				j++
			}
			i = j
		}
	}
	lit := s[start:i]
	if !isFloat {
		canon := lit
		if canon == "-0" {
			canon = "0"
		}
		return jval{kind: jInt, i: canon}, i, true
	}
	f, err := strconv.ParseFloat(lit, 64)
	if err != nil {
		return jval{}, start, false
	}
	return jval{kind: jFloat, f: f}, i, true
}

func scanJSONString(s string, i int) (string, int, error) {
	begin := i
	i++
	var raw []byte
	for {
		if i >= len(s) {
			return "", 0, jsonErr("Unterminated string starting at", s, begin)
		}
		c := s[i]
		if c == '"' {
			return string(raw), i + 1, nil
		}
		if c == '\\' {
			if i+1 >= len(s) {
				return "", 0, jsonErr("Unterminated string starting at", s, begin)
			}
			esc := s[i+1]
			if esc != 'u' {
				ch, ok := jsonSimpleEscape(esc)
				if !ok {
					return "", 0, jsonErr("Invalid \\escape", s, i)
				}
				raw = appendRuneWTF(raw, ch)
				i += 2
				continue
			}
			r, ni, err := decodeJSONU(s, i+1)
			if err != nil {
				return "", 0, jsonErr("Invalid \\uXXXX escape", s, i+1)
			}
			raw = appendRuneWTF(raw, r)
			i = ni
			continue
		}
		if c < 0x20 {
			return "", 0, jsonErr("Invalid control character at", s, i)
		}
		r, sz := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && sz == 1 {
			raw = append(raw, s[i])
			i++
			continue
		}
		raw = append(raw, s[i:i+sz]...)
		i += sz
	}
}

func jsonSimpleEscape(esc byte) (rune, bool) {
	switch esc {
	case '"', '\\', '/':
		return rune(esc), true
	case 'b':
		return '\b', true
	case 'f':
		return '\f', true
	case 'n':
		return '\n', true
	case 'r':
		return '\r', true
	case 't':
		return '\t', true
	default:
		return 0, false
	}
}

func decodeJSONU(s string, uPos int) (rune, int, error) {
	if uPos+4 >= len(s) && uPos+5 > len(s) {
		// need 4 hex digits after 'u'
	}
	if uPos+5 > len(s) {
		return 0, 0, jsonError{""}
	}
	hex := s[uPos+1 : uPos+5]
	v, ok := parseHex4(hex)
	if !ok {
		return 0, 0, jsonError{""}
	}
	next := uPos + 5
	if v >= 0xD800 && v <= 0xDBFF && next+6 <= len(s) && s[next] == '\\' && s[next+1] == 'u' {
		v2, ok2 := parseHex4(s[next+2 : next+6])
		if ok2 && v2 >= 0xDC00 && v2 <= 0xDFFF {
			combined := 0x10000 + (((v - 0xD800) << 10) | (v2 - 0xDC00))
			return rune(combined), next + 6, nil
		}
	}
	return rune(v), next, nil
}

func parseHex4(s string) (rune, bool) {
	if len(s) != 4 {
		return 0, false
	}
	var n rune
	for i := 0; i < 4; i++ {
		c := s[i]
		var d rune
		switch {
		case c >= '0' && c <= '9':
			d = rune(c - '0')
		case c >= 'a' && c <= 'f':
			d = rune(c-'a') + 10
		case c >= 'A' && c <= 'F':
			d = rune(c-'A') + 10
		default:
			return 0, false
		}
		n = n<<4 | d
	}
	return n, true
}

func appendRuneWTF(b []byte, r rune) []byte {
	if r >= 0xD800 && r <= 0xDFFF {
		return append(b,
			0xED,
			byte(0x80|((r>>6)&0x3F)),
			byte(0x80|(r&0x3F)),
		)
	}
	return utf8.AppendRune(b, r)
}

func mathNaN() float64 { return strconvNaN() }
func mathInf(sign int) float64 {
	f, _ := strconv.ParseFloat(map[int]string{1: "Inf", -1: "-Inf"}[sign], 64)
	return f
}

func strconvNaN() float64 {
	f, _ := strconv.ParseFloat("NaN", 64)
	return f
}
