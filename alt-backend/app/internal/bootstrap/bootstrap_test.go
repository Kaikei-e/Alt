package bootstrap

import "testing"

// Three processes emitting the same instrument names under one service.name
// collapse into a single meaningless series, so a mismatched OTEL_SERVICE_NAME
// is a startup failure rather than a warning (plan §3.2).
func TestResolveServiceName(t *testing.T) {
	tests := []struct {
		name    string
		env     string
		want    string
		expect  string
		wantErr bool
	}{
		{name: "unset falls back to the binary's own name", env: "", want: "alt-harvester", expect: "alt-harvester"},
		{name: "matching value is kept", env: "alt-data-hub", want: "alt-data-hub", expect: "alt-data-hub"},
		{name: "whitespace-only is treated as unset", env: "   ", want: "alt-backend", expect: "alt-backend"},
		{name: "leftover name from another binary fails", env: "alt-backend", want: "alt-harvester", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveServiceName(tt.env, tt.want)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("resolveServiceName(%q, %q) = %q, want error", tt.env, tt.want, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveServiceName(%q, %q) returned error: %v", tt.env, tt.want, err)
			}
			if got != tt.expect {
				t.Errorf("resolveServiceName(%q, %q) = %q, want %q", tt.env, tt.want, got, tt.expect)
			}
		})
	}
}
