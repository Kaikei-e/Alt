// Command protovis derives the public-API allowlists from the
// (alt.api.v1.visibility) option declared on each alt.* service.
//
// One declaration in proto/ becomes two generated gates: the BFF's Go map and
// the frontend's TypeScript tuple. A service that does not declare its
// visibility fails the run — there is no default, because a default here would
// silently decide whether something is browser-reachable.
//
// Usage:
//
//	cd proto && buf build --exclude-source-info -o /tmp/descriptor.binpb
//	cd tools/protovis && go run . -desc /tmp/descriptor.binpb -root ../../..
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

// Output paths, relative to the repo root.
const (
	defaultGoOut = "alt-butterfly-facade/internal/server/allowlist_gen.go"
	defaultTSOut = "alt-frontend-sv/src/lib/gen/allowlist.ts"
)

func main() {
	desc := flag.String("desc", "", "path to a FileDescriptorSet built by `buf build --exclude-source-info -o` (required)")
	root := flag.String("root", ".", "repo root that the output paths are resolved against")
	goOut := flag.String("go-out", defaultGoOut, "BFF allowlist output path, relative to -root")
	tsOut := flag.String("ts-out", defaultTSOut, "frontend allowlist output path, relative to -root")
	flag.Parse()

	if err := run(*desc, *root, *goOut, *tsOut); err != nil {
		var violations *ViolationError
		if errors.As(err, &violations) {
			for _, v := range violations.Violations {
				fmt.Fprintf(os.Stderr, "protovis: %s\n", v)
			}
			fmt.Fprintf(os.Stderr, "protovis: %d violation(s); no artifacts written\n", len(violations.Violations))
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "protovis: %v\n", err)
		os.Exit(1)
	}
}

func run(desc, root, goOut, tsOut string) error {
	if desc == "" {
		return errors.New("-desc is required")
	}

	fds, err := LoadDescriptorSet(desc)
	if err != nil {
		return err
	}

	list, err := Classify(fds)
	if err != nil {
		return err
	}

	goSrc, err := RenderGo(list)
	if err != nil {
		return err
	}
	tsSrc, err := RenderTS(list)
	if err != nil {
		return err
	}

	if err := writeArtifact(filepath.Join(root, goOut), goSrc); err != nil {
		return err
	}
	if err := writeArtifact(filepath.Join(root, tsOut), tsSrc); err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "protovis: %d public, %d admin -> %s, %s\n",
		len(list.Public), len(list.Admin), goOut, tsOut)
	return nil
}

func writeArtifact(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create output directory for %q: %w", path, err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return fmt.Errorf("write %q: %w", path, err)
	}
	return nil
}
