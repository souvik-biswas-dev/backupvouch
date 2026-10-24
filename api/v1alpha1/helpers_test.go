/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/util/validation/field"
)

func TestSetDefaults(t *testing.T) {
	t.Run("empty spec gets every default", func(t *testing.T) {
		s := BackupPolicySpec{Verify: VerifySpec{Checks: []Check{{Name: "a", SQL: "SELECT true"}}}}
		s.SetDefaults()
		if s.TimeZone != "UTC" || s.Source.Engine != EnginePostgres || s.Source.Port != 5432 {
			t.Errorf("source or time zone defaults missing: %+v", s)
		}
		if s.Retention.KeepLast != 7 || s.BackupTimeoutSeconds != 1800 || s.Verify.TimeoutSeconds != 900 {
			t.Errorf("numeric defaults missing: %+v", s)
		}
		if !s.Verify.IsEnabled() || s.Verify.Enabled == nil {
			t.Errorf("verify.enabled should default to true")
		}
		if s.Verify.ScratchSize == nil || s.Verify.ScratchSize.String() != "5Gi" {
			t.Errorf("scratchSize default wrong: %v", s.Verify.ScratchSize)
		}
		if s.Verify.Checks[0].Expect != "true" {
			t.Errorf("check expect default wrong: %q", s.Verify.Checks[0].Expect)
		}
	})
	t.Run("explicit values are kept", func(t *testing.T) {
		off := false
		s := BackupPolicySpec{
			TimeZone:             "Asia/Kolkata",
			BackupTimeoutSeconds: 60,
			Source:               SourceSpec{Port: 6543},
			Retention:            RetentionSpec{KeepLast: 3},
			Verify: VerifySpec{
				Enabled:        &off,
				TimeoutSeconds: 30,
				Checks:         []Check{{Name: "a", SQL: "SELECT 1", Expect: "1"}},
			},
		}
		s.SetDefaults()
		if s.TimeZone != "Asia/Kolkata" || s.BackupTimeoutSeconds != 60 || s.Source.Port != 6543 ||
			s.Retention.KeepLast != 3 || s.Verify.TimeoutSeconds != 30 || s.Verify.Checks[0].Expect != "1" {
			t.Errorf("explicit values were overwritten: %+v", s)
		}
		if s.Verify.IsEnabled() {
			t.Errorf("explicit verify.enabled=false must stay false")
		}
	})
}

func TestVerifyIsEnabled(t *testing.T) {
	yes, no := true, false
	tests := []struct {
		name string
		in   *bool
		want bool
	}{
		{"unset means enabled", nil, true},
		{"true", &yes, true},
		{"false", &no, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (VerifySpec{Enabled: tt.in}).IsEnabled(); got != tt.want {
				t.Errorf("IsEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateCheckSQL(t *testing.T) {
	tests := []struct {
		name    string
		sql     string
		wantErr string // substring, empty means valid
	}{
		{"simple", "SELECT count(*) > 0 FROM users", ""},
		{"trailing semicolon", "SELECT 1;", ""},
		{"trailing semicolon and spaces", "SELECT 1 ;  \n", ""},
		{"semicolon inside string", "SELECT ';' = ';'", ""},
		{"escaped quote then semicolon in string", "SELECT 'a'';b'", ""},
		{"semicolon in quoted identifier", `SELECT 1 FROM "a;b"`, ""},
		{"line comment after statement", "SELECT 1; -- done", ""},
		{"block comment with semicolon", "SELECT 1 /* ; */", ""},
		{"empty", "", "empty"},
		{"only spaces", "   \n", "empty"},
		{"only a semicolon", ";", "no statement"},
		{"only a comment", "-- nothing", "no statement"},
		{"two statements", "SELECT 1; SELECT 2", "single statement"},
		{"statement then drop", "SELECT 1; DROP TABLE users", "single statement"},
		{"string after terminator", "SELECT 1; 'x'", "single statement"},
		{"unterminated string", "SELECT 'abc", "unterminated quote"},
		{"unterminated comment", "SELECT 1 /* abc", "unterminated comment"},
		{"too long", "SELECT '" + strings.Repeat("a", MaxCheckSQLLength) + "'", "at most"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCheckSQL(tt.sql)
			switch {
			case tt.wantErr == "" && err != nil:
				t.Errorf("unexpected error: %v", err)
			case tt.wantErr != "" && err == nil:
				t.Errorf("expected error containing %q, got nil", tt.wantErr)
			case tt.wantErr != "" && !strings.Contains(err.Error(), tt.wantErr):
				t.Errorf("error %q does not contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestValidateChecks(t *testing.T) {
	many := make([]Check, MaxChecks+1)
	for i := range many {
		many[i] = Check{Name: "c" + strings.Repeat("x", i), SQL: "SELECT true"}
	}
	tests := []struct {
		name      string
		checks    []Check
		wantPaths []string // field paths that must have an error; nil means valid
	}{
		{"none", nil, nil},
		{"one valid", []Check{{Name: "users-not-empty", SQL: "SELECT count(*) > 0 FROM users"}}, nil},
		{"two valid", []Check{{Name: "a", SQL: "SELECT true"}, {Name: "b1", SQL: "SELECT true"}}, nil},
		{"missing name", []Check{{SQL: "SELECT true"}}, []string{"checks[0].name"}},
		{"upper case name", []Check{{Name: "Users", SQL: "SELECT true"}}, []string{"checks[0].name"}},
		{"name starts with dash", []Check{{Name: "-a", SQL: "SELECT true"}}, []string{"checks[0].name"}},
		{"name too long", []Check{{Name: strings.Repeat("a", 64), SQL: "SELECT true"}}, []string{"checks[0].name"}},
		{"duplicate name", []Check{{Name: "a", SQL: "SELECT true"}, {Name: "a", SQL: "SELECT true"}}, []string{"checks[1].name"}},
		{"empty sql", []Check{{Name: "a", SQL: ""}}, []string{"checks[0].sql"}},
		{"two statements", []Check{{Name: "a", SQL: "SELECT 1; SELECT 2"}}, []string{"checks[0].sql"}},
		{"too many checks", many, []string{"checks"}},
		{"several problems", []Check{{Name: "A", SQL: ""}}, []string{"checks[0].name", "checks[0].sql"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateChecks(tt.checks, field.NewPath("checks"))
			got := map[string]bool{}
			for _, e := range errs {
				got[e.Field] = true
			}
			if len(tt.wantPaths) == 0 {
				if len(errs) != 0 {
					t.Fatalf("expected no errors, got %v", errs)
				}
				return
			}
			for _, p := range tt.wantPaths {
				if !got[p] {
					t.Errorf("expected an error at %q, got %v", p, errs)
				}
			}
		})
	}
}

func TestValidateTimeZone(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{"UTC", "UTC", false},
		{"Asia/Kolkata", "Asia/Kolkata", false},
		{"America/New_York", "America/New_York", false},
		{"empty", "", true},
		{"Local", "Local", true},
		{"lower case local", "local", true},
		{"unknown", "Mars/Olympus", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTimeZone(tt.in)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTimeZone(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
		})
	}
}
