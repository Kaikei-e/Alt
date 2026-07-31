package datahubapi

import (
	"fmt"
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"

	datahubv1 "alt/gen/proto/alt/datahub/v1"
	backendv1 "alt/gen/proto/services/backend/v1"
)

// absorbedRESTProcedures are the two DataHubService procedures that have no
// BackendInternalService counterpart: they replace REST routes, not RPCs
// (ADR-000954 D6). Everything else must exist on both services and must be
// byte-for-byte interchangeable.
var absorbedRESTProcedures = map[string]string{
	"GetSystemUser":      "GET /v1/internal/system-user",
	"ListRecentArticles": "GET /v1/internal/articles/recent",
}

func datahubService(t *testing.T) protoreflect.ServiceDescriptor {
	t.Helper()
	svcs := datahubv1.File_alt_datahub_v1_datahub_proto.Services()
	for i := range svcs.Len() {
		if svcs.Get(i).Name() == "DataHubService" {
			return svcs.Get(i)
		}
	}
	t.Fatal("alt.datahub.v1.DataHubService not found in the generated file descriptor")
	return nil
}

func backendInternalService(t *testing.T) protoreflect.ServiceDescriptor {
	t.Helper()
	svcs := backendv1.File_services_backend_v1_internal_proto.Services()
	for i := range svcs.Len() {
		if svcs.Get(i).Name() == "BackendInternalService" {
			return svcs.Get(i)
		}
	}
	t.Fatal("services.backend.v1.BackendInternalService not found in the generated file descriptor")
	return nil
}

// TestDataHubServiceCoversEveryLegacyProcedure pins the migration's central
// claim: the new namespace is a superset of the old one, procedure for
// procedure. Wave 2-B moves peers one at a time on the strength of that, so a
// procedure quietly missing here would not be found until a peer's PR failed.
func TestDataHubServiceCoversEveryLegacyProcedure(t *testing.T) {
	newSvc, oldSvc := datahubService(t), backendInternalService(t)

	newNames := map[string]bool{}
	for i := range newSvc.Methods().Len() {
		newNames[string(newSvc.Methods().Get(i).Name())] = true
	}

	for i := range oldSvc.Methods().Len() {
		name := string(oldSvc.Methods().Get(i).Name())
		if !newNames[name] {
			t.Errorf("alt.datahub.v1.DataHubService is missing %s, which services.backend.v1.BackendInternalService serves", name)
		}
		delete(newNames, name)
	}

	for name := range newNames {
		if _, ok := absorbedRESTProcedures[name]; !ok {
			t.Errorf("DataHubService.%s has no BackendInternalService counterpart and is not a recorded absorbed REST route; "+
				"if it is a genuinely new capability, add it to absorbedRESTProcedures with its origin", name)
		}
	}
}

// TestEveryMigratedProcedureIsWireIdentical walks both descriptor trees field
// by field.
//
// Wave 2-B is a URL change and nothing else — a peer keeps its serialiser, its
// field names and its numbers, and only the path it POSTs to moves. That is
// only true while the two message trees agree on every field number, name,
// type, cardinality and presence rule. The adapter in handler.go re-encodes
// requests by marshalling to bytes and unmarshalling into the other type, so a
// divergence here would not raise an error at runtime: protobuf keeps
// unrecognised fields aside and hands back a message that is simply missing
// data. This test is what makes that failure loud instead.
func TestEveryMigratedProcedureIsWireIdentical(t *testing.T) {
	newSvc, oldSvc := datahubService(t), backendInternalService(t)

	for i := range oldSvc.Methods().Len() {
		legacy := oldSvc.Methods().Get(i)
		name := string(legacy.Name())

		current := newSvc.Methods().ByName(legacy.Name())
		if current == nil {
			continue // reported by TestDataHubServiceCoversEveryLegacyProcedure
		}

		t.Run(name, func(t *testing.T) {
			assertWireIdentical(t, name+" request", legacy.Input(), current.Input(), map[string]bool{})
			assertWireIdentical(t, name+" response", legacy.Output(), current.Output(), map[string]bool{})
		})
	}
}

// assertWireIdentical compares two message descriptors as protobuf wire
// schemas. Message and field *type* names are allowed to differ — those never
// travel on the wire — but everything a decoder consults must match.
func assertWireIdentical(t *testing.T, path string, want, got protoreflect.MessageDescriptor, seen map[string]bool) {
	t.Helper()

	// Same descriptor on both sides (google.protobuf.Timestamp and friends).
	if want.FullName() == got.FullName() {
		return
	}
	key := string(want.FullName()) + "|" + string(got.FullName())
	if seen[key] {
		return
	}
	seen[key] = true

	if want.Fields().Len() != got.Fields().Len() {
		t.Errorf("%s: field count %s=%d %s=%d",
			path, want.FullName(), want.Fields().Len(), got.FullName(), got.Fields().Len())
	}

	for i := range want.Fields().Len() {
		wf := want.Fields().Get(i)
		gf := got.Fields().ByNumber(wf.Number())
		if gf == nil {
			t.Errorf("%s: %s has no field number %d (%s in %s)",
				path, got.FullName(), wf.Number(), wf.Name(), want.FullName())
			continue
		}

		fieldPath := fmt.Sprintf("%s.%s(#%d)", path, wf.Name(), wf.Number())

		// Field names ride the wire in Connect's JSON codec, which every
		// existing peer uses, so a rename is as breaking as a renumber.
		if wf.Name() != gf.Name() {
			t.Errorf("%s: name %q vs %q", fieldPath, wf.Name(), gf.Name())
		}
		if wf.JSONName() != gf.JSONName() {
			t.Errorf("%s: JSON name %q vs %q", fieldPath, wf.JSONName(), gf.JSONName())
		}
		if wf.Kind() != gf.Kind() {
			t.Errorf("%s: kind %v vs %v", fieldPath, wf.Kind(), gf.Kind())
		}
		if wf.Cardinality() != gf.Cardinality() {
			t.Errorf("%s: cardinality %v vs %v", fieldPath, wf.Cardinality(), gf.Cardinality())
		}
		// Explicit presence decides whether a zero value is transmitted at
		// all, which is the difference between "limit 0" and "no limit".
		if wf.HasPresence() != gf.HasPresence() {
			t.Errorf("%s: explicit presence %v vs %v", fieldPath, wf.HasPresence(), gf.HasPresence())
		}
		if wf.IsList() != gf.IsList() || wf.IsMap() != gf.IsMap() {
			t.Errorf("%s: list/map shape (%v,%v) vs (%v,%v)",
				fieldPath, wf.IsList(), wf.IsMap(), gf.IsList(), gf.IsMap())
		}

		if wf.Kind() == protoreflect.MessageKind || wf.Kind() == protoreflect.GroupKind {
			if gf.Kind() != wf.Kind() {
				continue
			}
			assertWireIdentical(t, fieldPath, wf.Message(), gf.Message(), seen)
		}
	}

	for i := range got.Fields().Len() {
		gf := got.Fields().Get(i)
		if want.Fields().ByNumber(gf.Number()) == nil {
			t.Errorf("%s: %s has extra field %s(#%d) that %s does not",
				path, got.FullName(), gf.Name(), gf.Number(), want.FullName())
		}
	}
}
