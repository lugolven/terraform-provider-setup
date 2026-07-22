package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestResolveMaxConcurrentSessions(t *testing.T) {
	tests := []struct {
		name     string
		cfg      types.Int64
		env      *string // nil => unset
		want     int
		wantFail bool
	}{
		{
			name: "default when null and no env",
			cfg:  types.Int64Null(),
			want: defaultMaxConcurrentSessions,
		},
		{
			name: "config value takes precedence",
			cfg:  types.Int64Value(8),
			want: 8,
		},
		{
			name: "zero from config disables the limit",
			cfg:  types.Int64Value(0),
			want: 0,
		},
		{
			name: "env used when config null",
			cfg:  types.Int64Null(),
			env:  strPtr("3"),
			want: 3,
		},
		{
			name: "config wins over env",
			cfg:  types.Int64Value(7),
			env:  strPtr("3"),
			want: 7,
		},
		{
			name:     "negative config rejected",
			cfg:      types.Int64Value(-1),
			wantFail: true,
		},
		{
			name:     "unparseable env rejected",
			cfg:      types.Int64Null(),
			env:      strPtr("not-a-number"),
			wantFail: true,
		},
		{
			name:     "negative env rejected",
			cfg:      types.Int64Null(),
			env:      strPtr("-4"),
			wantFail: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			if tt.env != nil {
				t.Setenv(maxConcurrentSessionsEnvVar, *tt.env)
			} else {
				// Ensure a value leaking in from the environment does not affect the
				// "no env" cases.
				t.Setenv(maxConcurrentSessionsEnvVar, "")
			}

			resp := &provider.ConfigureResponse{}

			// Act
			got, ok := resolveMaxConcurrentSessions(tt.cfg, resp)

			// Assert
			if tt.wantFail {
				if ok {
					t.Fatalf("expected failure, got value %d", got)
				}

				if !resp.Diagnostics.HasError() {
					t.Fatal("expected a diagnostic error to be reported")
				}

				return
			}

			if !ok {
				t.Fatalf("unexpected failure: %v", resp.Diagnostics.Errors())
			}

			if got != tt.want {
				t.Fatalf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func strPtr(s string) *string {
	return &s
}
