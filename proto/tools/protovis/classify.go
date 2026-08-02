package main

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"

	apiv1 "protovis/gen/alt/api/v1"
)

// The two package roots the repo recognises. alt.* is north-south (browser
// facing, reachable through the BFF); services.* is east-west (mTLS between
// backends). Anything else is an unclassified namespace and is treated as a
// declaration bug rather than silently ignored.
const (
	northSouthRoot = "alt"
	eastWestRoot   = "services"
)

// Allowlist is the classified public surface, sorted by fully qualified
// service name so the emitted artifacts are byte-stable.
type Allowlist struct {
	Public []string
	Admin  []string
}

// ViolationError carries every declaration problem found in one pass. The tool
// reports all of them at once: fixing one annotation at a time across 16
// services would be a poor gate.
type ViolationError struct {
	Violations []string
}

func (e *ViolationError) Error() string {
	return fmt.Sprintf("%d service visibility violation(s):\n  %s",
		len(e.Violations), strings.Join(e.Violations, "\n  "))
}

// LoadDescriptorSet reads a FileDescriptorSet produced by
// `buf build --exclude-source-info -o <path>`. The set must include imports
// (never pass --exclude-imports), because protodesc resolves every dependency.
func LoadDescriptorSet(path string) (*descriptorpb.FileDescriptorSet, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read descriptor set %q: %w", path, err)
	}

	fds := &descriptorpb.FileDescriptorSet{}
	// The default resolver is protoregistry.GlobalTypes, where the linked
	// apiv1 package has registered E_Visibility. That is what makes the
	// option come back as a typed extension instead of unknown bytes.
	if err := proto.Unmarshal(raw, fds); err != nil {
		return nil, fmt.Errorf("parse descriptor set %q: %w", path, err)
	}
	return fds, nil
}

// Classify partitions every service in the set into the public and admin
// allowlists, and reports every service whose declaration does not fit the
// namespace convention.
func Classify(fds *descriptorpb.FileDescriptorSet) (Allowlist, error) {
	files, err := protodesc.NewFiles(fds)
	if err != nil {
		return Allowlist{}, fmt.Errorf("build file registry: %w", err)
	}

	var list Allowlist
	var violations []string

	files.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
		services := fd.Services()
		for i := range services.Len() {
			svc := services.Get(i)
			name := string(svc.FullName())

			switch root, _, _ := strings.Cut(string(fd.Package()), "."); root {
			case northSouthRoot:
				switch vis, declared := visibilityOf(svc); {
				case !declared:
					violations = append(violations, fmt.Sprintf(
						"%s: no (alt.api.v1.visibility) option; every alt.* service must declare VISIBILITY_PUBLIC or VISIBILITY_ADMIN", name))
				case vis == apiv1.Visibility_VISIBILITY_PUBLIC:
					list.Public = append(list.Public, name)
				case vis == apiv1.Visibility_VISIBILITY_ADMIN:
					list.Admin = append(list.Admin, name)
				default:
					violations = append(violations, fmt.Sprintf(
						"%s: (alt.api.v1.visibility) is %s; declare VISIBILITY_PUBLIC or VISIBILITY_ADMIN", name, vis))
				}
			case eastWestRoot:
				if _, declared := visibilityOf(svc); declared {
					violations = append(violations, fmt.Sprintf(
						"%s: services.* is the east-west root and must not carry (alt.api.v1.visibility)", name))
				}
			default:
				violations = append(violations, fmt.Sprintf(
					"%s: unknown package root %q; only alt.* (north-south) and services.* (east-west) exist", name, root))
			}
		}
		return true
	})

	slices.Sort(list.Public)
	slices.Sort(list.Admin)
	slices.Sort(violations)

	if len(violations) > 0 {
		return Allowlist{}, &ViolationError{Violations: violations}
	}
	return list, nil
}

// visibilityOf reads the option off a service. The second result distinguishes
// "no option at all" from "option present but UNSPECIFIED" — both are errors on
// an alt.* service, but only the first is an error on services.*.
func visibilityOf(svc protoreflect.ServiceDescriptor) (apiv1.Visibility, bool) {
	opts, ok := svc.Options().(*descriptorpb.ServiceOptions)
	if !ok || opts == nil {
		return apiv1.Visibility_VISIBILITY_UNSPECIFIED, false
	}
	if !proto.HasExtension(opts, apiv1.E_Visibility) {
		return apiv1.Visibility_VISIBILITY_UNSPECIFIED, false
	}
	vis, ok := proto.GetExtension(opts, apiv1.E_Visibility).(apiv1.Visibility)
	if !ok {
		return apiv1.Visibility_VISIBILITY_UNSPECIFIED, false
	}
	return vis, true
}
