package screener

// PyTokKind mirrors the internal Python token kinds for callers like internal/packet.
type PyTokKind int

const (
	PyEOF PyTokKind = iota
	PyNL
	PyIndent
	PyDedent
	PyIdent
	PyNum
	PyStr
	PyOp
)

// PyToken is an exported Python lexical token produced by TokenizePython.
type PyToken struct {
	Kind  PyTokKind
	Val   string
	Exprs []string
	Line  int
}

// TokenizePython lexes Python 3.12-3.14 source (including PEP 701 f-strings and INDENT/DEDENT).
func TokenizePython(src string) ([]PyToken, error) {
	raw, err := tokenizePy(src)
	if err != nil {
		return nil, err
	}
	return exportPyToks(raw), nil
}

// TokenizePythonExpr lexes a Python expression fragment without INDENT/DEDENT/NL tracking.
func TokenizePythonExpr(src string) ([]PyToken, error) {
	raw, err := tokenizePyExpr(src)
	if err != nil {
		return nil, err
	}
	return exportPyToks(raw), nil
}

// ValidatePythonSyntax checks Python syntax and indentation structure using screener's parser.
func ValidatePythonSyntax(src string) error {
	_, err := parsePython(src)
	return err
}

func exportPyToks(raw []pyTok) []PyToken {
	out := make([]PyToken, len(raw))
	for i, tk := range raw {
		out[i] = PyToken{
			Kind:  PyTokKind(tk.kind),
			Val:   tk.val,
			Exprs: tk.exprs,
			Line:  tk.line,
		}
	}
	return out
}
