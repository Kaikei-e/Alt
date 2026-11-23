package ratchet

import (
	"fmt"
	"os"
	"strings"
	"syscall"
)

type seenEnt struct {
	resolved bool
	path     string
}

type loopError struct{ path string }

func (e loopError) Error() string {
	return "Symlink loop from " + pyReprString(e.path)
}

func resolvePath(filename string) (string, error) {
	if filename == "" {
		filename = ""
	}
	path, _ := joinRealpath("", filename, map[string]seenEnt{})
	path = absPath(path)
	if err := statLoop(path); err != nil {
		return "", err
	}
	return path, nil
}

func statLoop(path string) error {
	_, err := os.Stat(path)
	if err == nil {
		return nil
	}
	if errno, ok := asErrno(err); ok && errno == syscall.ELOOP {
		return loopError{path: path}
	}
	return nil
}

func asErrno(err error) (syscall.Errno, bool) {
	if err == nil {
		return 0, false
	}
	pe, ok := err.(*os.PathError)
	if !ok {
		return 0, false
	}
	errno, ok := pe.Err.(syscall.Errno)
	return errno, ok
}

func joinRealpath(path, rest string, seen map[string]seenEnt) (string, bool) {
	const sep = "/"
	if strings.HasPrefix(rest, sep) {
		rest = rest[1:]
		path = sep
	}
	for rest != "" {
		name, tail, found := strings.Cut(rest, sep)
		if found {
			rest = tail
		} else {
			rest = ""
		}
		if name == "" || name == "." {
			continue
		}
		if name == ".." {
			if path != "" {
				var last string
				path, last = splitPath(path)
				if last == ".." {
					path = pyJoin(path, "..", "..")
				}
			} else {
				path = ".."
			}
			continue
		}
		newpath := pyJoin(path, name)
		fi, err := os.Lstat(newpath)
		isLink := err == nil && fi.Mode()&os.ModeSymlink != 0
		if !isLink {
			path = newpath
			continue
		}
		if ent, ok := seen[newpath]; ok {
			if ent.resolved {
				path = ent.path
				continue
			}
			return pyJoin(newpath, rest), false
		}
		seen[newpath] = seenEnt{}
		target, rerr := os.Readlink(newpath)
		if rerr != nil {
			return pyJoin(newpath, rest), false
		}
		var ok bool
		path, ok = joinRealpath(path, target, seen)
		if !ok {
			return pyJoin(path, rest), false
		}
		seen[newpath] = seenEnt{resolved: true, path: path}
	}
	return path, true
}

func pyJoin(elem ...string) string {
	path := ""
	first := true
	for _, b := range elem {
		if first {
			path = b
			first = false
			continue
		}
		if strings.HasPrefix(b, "/") {
			path = b
			continue
		}
		if path == "" || strings.HasSuffix(path, "/") {
			path += b
			continue
		}
		path += "/" + b
	}
	return path
}

func splitPath(p string) (string, string) {
	i := strings.LastIndex(p, "/") + 1
	head, tail := p[:i], p[i:]
	if head != "" && strings.Trim(head, "/") != "" {
		head = strings.TrimRight(head, "/")
	}
	return head, tail
}

func absPath(path string) string {
	if !strings.HasPrefix(path, "/") {
		cwd, err := os.Getwd()
		if err != nil {
			cwd = "."
		}
		path = pyJoin(cwd, path)
	}
	return normPath(path)
}

func normPath(path string) string {
	if path == "" {
		return "."
	}
	initial := 0
	if strings.HasPrefix(path, "/") {
		initial = 1
		if strings.HasPrefix(path, "//") && !strings.HasPrefix(path, "///") {
			initial = 2
		}
	}
	var comps []string
	for _, comp := range strings.Split(path, "/") {
		if comp == "" || comp == "." {
			continue
		}
		if comp != ".." || (initial == 0 && len(comps) == 0) || (len(comps) > 0 && comps[len(comps)-1] == "..") {
			comps = append(comps, comp)
			continue
		}
		if len(comps) > 0 {
			comps = comps[:len(comps)-1]
		}
	}
	out := strings.Join(comps, "/")
	if initial > 0 {
		out = strings.Repeat("/", initial) + out
	}
	if out == "" {
		return "."
	}
	return out
}

func isSymlink(path string) bool {
	fi, err := os.Lstat(path)
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeSymlink != 0
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func isRegularFile(path string) bool {
	fi, err := os.Stat(path)
	if err != nil {
		return false
	}
	return fi.Mode().IsRegular()
}

func isDir(path string) bool {
	fi, err := os.Stat(path)
	if err != nil {
		return false
	}
	return fi.IsDir()
}

func relTo(root, full string) (string, bool) {
	root = strings.TrimRight(normPath(root), "/")
	full = normPath(full)
	if root == "/" {
		if full == "/" {
			return ".", true
		}
		if strings.HasPrefix(full, "/") {
			return strings.TrimPrefix(full, "/"), true
		}
		return "", false
	}
	if full == root {
		return ".", true
	}
	prefix := root + "/"
	if strings.HasPrefix(full, prefix) {
		return strings.TrimPrefix(full, prefix), true
	}
	return "", false
}

func pyOSError(err error) string {
	if err == nil {
		return ""
	}
	if le, ok := err.(loopError); ok {
		return le.Error()
	}
	if pe, ok := err.(*os.PathError); ok {
		if errno, ok := pe.Err.(syscall.Errno); ok {
			return fmt.Sprintf("[Errno %d] %s: %s", int(errno), pyStrError(errno), pyReprString(pe.Path))
		}
	}
	return err.Error()
}

func pyStrError(errno syscall.Errno) string {
	msg := errno.Error()
	if msg == "" {
		return msg
	}
	r := []rune(msg)
	r[0] = []rune(strings.ToUpper(string(r[0])))[0]
	return string(r)
}

func trimLeftSet(s, cut string) string {
	i := 0
	for i < len(s) {
		r, sz := decodeWTF8(s, i)
		if !strings.ContainsRune(cut, r) {
			break
		}
		i += sz
	}
	return s[i:]
}

func hasDotDot(path string) bool {
	for _, part := range strings.Split(path, "/") {
		if part == ".." {
			return true
		}
	}
	return false
}
