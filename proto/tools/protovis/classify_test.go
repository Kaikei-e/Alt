package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/types/descriptorpb"

	apiv1 "protovis/gen/alt/api/v1"
)

// service builds a ServiceDescriptorProto, optionally carrying the visibility
// option. A nil vis means the option is absent entirely, which is a different
// state from VISIBILITY_UNSPECIFIED and must be reported differently.
func service(name string, vis *apiv1.Visibility) *descriptorpb.ServiceDescriptorProto {
	svc := &descriptorpb.ServiceDescriptorProto{Name: proto.String(name)}
	if vis != nil {
		opts := &descriptorpb.ServiceOptions{}
		proto.SetExtension(opts, apiv1.E_Visibility, *vis)
		svc.Options = opts
	}
	return svc
}

func visibility(v apiv1.Visibility) *apiv1.Visibility { return &v }

// file builds a FileDescriptorProto at the canonical path for its package, so
// that "alt.demo.v1" lands at "alt/demo/v1/demo.proto".
func file(pkg string, services ...*descriptorpb.ServiceDescriptorProto) *descriptorpb.FileDescriptorProto {
	path := filepath.Join(filepath.Join(strings.Split(pkg, ".")...), "demo.proto")
	return &descriptorpb.FileDescriptorProto{
		Name:       proto.String(path),
		Package:    proto.String(pkg),
		Syntax:     proto.String("proto3"),
		Dependency: []string{"alt/api/v1/visibility.proto"},
		Service:    services,
	}
}

// descriptorSetFile marshals files (plus the option proto and its own
// dependency) to a temp .binpb, mirroring what `buf build` hands the tool.
func descriptorSetFile(t *testing.T, files ...*descriptorpb.FileDescriptorProto) string {
	t.Helper()

	fds := &descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{
			protodesc.ToFileDescriptorProto(descriptorpb.File_google_protobuf_descriptor_proto),
			protodesc.ToFileDescriptorProto(apiv1.File_alt_api_v1_visibility_proto),
		},
	}
	fds.File = append(fds.File, files...)

	raw, err := proto.Marshal(fds)
	if err != nil {
		t.Fatalf("marshal descriptor set: %v", err)
	}
	path := filepath.Join(t.TempDir(), "descriptor.binpb")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write descriptor set: %v", err)
	}
	return path
}

func TestClassify(t *testing.T) {
	tests := []struct {
		name       string
		files      []*descriptorpb.FileDescriptorProto
		want       Allowlist
		violations []string
	}{
		{
			name: "alt services are split by visibility and east-west services are ignored",
			files: []*descriptorpb.FileDescriptorProto{
				file("alt.demo.v1",
					service("DemoService", visibility(apiv1.Visibility_VISIBILITY_PUBLIC)),
					service("DemoAdminService", visibility(apiv1.Visibility_VISIBILITY_ADMIN)),
				),
				file("services.east.v1", service("EastService", nil)),
			},
			want: Allowlist{
				Public: []string{"alt.demo.v1.DemoService"},
				Admin:  []string{"alt.demo.v1.DemoAdminService"},
			},
		},
		{
			name: "alt service without the option is rejected",
			files: []*descriptorpb.FileDescriptorProto{
				file("alt.demo.v1", service("DemoService", nil)),
			},
			violations: []string{
				"alt.demo.v1.DemoService: no (alt.api.v1.visibility) option; every alt.* service must declare VISIBILITY_PUBLIC or VISIBILITY_ADMIN",
			},
		},
		{
			name: "alt service declaring UNSPECIFIED is rejected",
			files: []*descriptorpb.FileDescriptorProto{
				file("alt.demo.v1", service("DemoService", visibility(apiv1.Visibility_VISIBILITY_UNSPECIFIED))),
			},
			violations: []string{
				"alt.demo.v1.DemoService: (alt.api.v1.visibility) is VISIBILITY_UNSPECIFIED; declare VISIBILITY_PUBLIC or VISIBILITY_ADMIN",
			},
		},
		{
			name: "east-west service carrying the option is rejected",
			files: []*descriptorpb.FileDescriptorProto{
				file("services.east.v1", service("EastService", visibility(apiv1.Visibility_VISIBILITY_PUBLIC))),
			},
			violations: []string{
				"services.east.v1.EastService: services.* is the east-west root and must not carry (alt.api.v1.visibility)",
			},
		},
		{
			name: "service outside the two known roots is rejected",
			files: []*descriptorpb.FileDescriptorProto{
				file("internal.demo.v1", service("DemoService", nil)),
			},
			violations: []string{
				`internal.demo.v1.DemoService: unknown package root "internal"; only alt.* (north-south) and services.* (east-west) exist`,
			},
		},
		{
			name: "every violation is reported, not only the first",
			files: []*descriptorpb.FileDescriptorProto{
				file("alt.demo.v1",
					service("BetaService", nil),
					service("AlphaService", visibility(apiv1.Visibility_VISIBILITY_UNSPECIFIED)),
				),
			},
			violations: []string{
				"alt.demo.v1.AlphaService: (alt.api.v1.visibility) is VISIBILITY_UNSPECIFIED; declare VISIBILITY_PUBLIC or VISIBILITY_ADMIN",
				"alt.demo.v1.BetaService: no (alt.api.v1.visibility) option; every alt.* service must declare VISIBILITY_PUBLIC or VISIBILITY_ADMIN",
			},
		},
		{
			name: "output is sorted regardless of declaration order",
			files: []*descriptorpb.FileDescriptorProto{
				file("alt.zulu.v1",
					service("ZuluService", visibility(apiv1.Visibility_VISIBILITY_PUBLIC)),
					service("AlphaService", visibility(apiv1.Visibility_VISIBILITY_PUBLIC)),
				),
				file("alt.alpha.v1", service("MidService", visibility(apiv1.Visibility_VISIBILITY_PUBLIC))),
			},
			want: Allowlist{
				Public: []string{
					"alt.alpha.v1.MidService",
					"alt.zulu.v1.AlphaService",
					"alt.zulu.v1.ZuluService",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fds, err := LoadDescriptorSet(descriptorSetFile(t, tt.files...))
			if err != nil {
				t.Fatalf("LoadDescriptorSet() error = %v", err)
			}

			got, err := Classify(fds)

			if len(tt.violations) > 0 {
				var verr *ViolationError
				if !errors.As(err, &verr) {
					t.Fatalf("Classify() error = %v, want *ViolationError", err)
				}
				assertEqualStrings(t, "violations", verr.Violations, tt.violations)
				return
			}
			if err != nil {
				t.Fatalf("Classify() error = %v", err)
			}
			assertEqualStrings(t, "public", got.Public, tt.want.Public)
			assertEqualStrings(t, "admin", got.Admin, tt.want.Admin)
		})
	}
}

func assertEqualStrings(t *testing.T, label string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s = %v, want %v", label, got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("%s = %v, want %v", label, got, want)
		}
	}
}
