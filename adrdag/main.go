// adrdag derives the currently-binding architecture decisions from the ADR
// supersedes DAG under docs/ADR. Semantics-compatible successor to
// scripts/adr_graph.py.
package main

import (
	"os"

	"github.com/alt-project/adrdag/cmd"
)

func main() {
	os.Exit(cmd.Execute())
}
