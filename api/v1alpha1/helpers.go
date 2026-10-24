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
	"fmt"
	"regexp"
	"strings"
	"time"
	_ "time/tzdata" // embed the time zone database so validation works in minimal images

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

// Defaults and limits. They mirror the kubebuilder markers in
// backuppolicy_types.go and docs/API.md.
const (
	DefaultTimeZone                   = "UTC"
	DefaultPostgresPort         int32 = 5432
	DefaultKeepLast             int32 = 7
	DefaultBackupTimeoutSeconds int32 = 1800
	DefaultVerifyTimeoutSeconds int32 = 900
	DefaultScratchSize                = "5Gi"
	DefaultCheckExpect                = "true"

	MaxChecks           = 20
	MaxCheckSQLLength   = 2000
	MaxPolicyNameLength = 45
)

var checkNameRE = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

// SetDefaults fills in the defaults that the API server applies from the CRD
// schema. The controller calls it so it never depends on whether the object
// came from a server that applied defaults (for example in unit tests).
func (s *BackupPolicySpec) SetDefaults() {
	if s.TimeZone == "" {
		s.TimeZone = DefaultTimeZone
	}
	if s.Source.Engine == "" {
		s.Source.Engine = EnginePostgres
	}
	if s.Source.Port == 0 {
		s.Source.Port = DefaultPostgresPort
	}
	if s.Retention.KeepLast == 0 {
		s.Retention.KeepLast = DefaultKeepLast
	}
	if s.BackupTimeoutSeconds == 0 {
		s.BackupTimeoutSeconds = DefaultBackupTimeoutSeconds
	}
	if s.Verify.Enabled == nil {
		enabled := true
		s.Verify.Enabled = &enabled
	}
	if s.Verify.TimeoutSeconds == 0 {
		s.Verify.TimeoutSeconds = DefaultVerifyTimeoutSeconds
	}
	if s.Verify.ScratchSize == nil {
		size := resource.MustParse(DefaultScratchSize)
		s.Verify.ScratchSize = &size
	}
	for i := range s.Verify.Checks {
		if s.Verify.Checks[i].Expect == "" {
			s.Verify.Checks[i].Expect = DefaultCheckExpect
		}
	}
}

// IsEnabled reports whether verification is on. An unset value means enabled.
func (v VerifySpec) IsEnabled() bool {
	return v.Enabled == nil || *v.Enabled
}

// ValidateChecks validates the check definitions: at most MaxChecks entries,
// valid unique names, and a single-statement SQL text for each.
// The API server already enforces most of this through the CRD schema; this
// function lets the controller and the runner re-check defensively.
func ValidateChecks(checks []Check, fldPath *field.Path) field.ErrorList {
	var errs field.ErrorList
	if len(checks) > MaxChecks {
		errs = append(errs, field.Invalid(fldPath, len(checks),
			fmt.Sprintf("at most %d checks are allowed", MaxChecks)))
	}
	seen := make(map[string]struct{}, len(checks))
	for i, c := range checks {
		p := fldPath.Index(i)
		switch {
		case c.Name == "":
			errs = append(errs, field.Required(p.Child("name"), "name is required"))
		case len(c.Name) > 63 || !checkNameRE.MatchString(c.Name):
			errs = append(errs, field.Invalid(p.Child("name"), c.Name,
				"must be at most 63 characters of lower case letters, digits and '-', starting and ending with a letter or digit"))
		default:
			if _, dup := seen[c.Name]; dup {
				errs = append(errs, field.Duplicate(p.Child("name"), c.Name))
			}
			seen[c.Name] = struct{}{}
		}
		if err := ValidateCheckSQL(c.SQL); err != nil {
			errs = append(errs, field.Invalid(p.Child("sql"), "<sql>", err.Error()))
		}
	}
	return errs
}

// ValidateCheckSQL checks that sql is non-empty, not too long, and a single
// statement (an optional trailing semicolon is allowed).
//
// This is a best-effort scan that understands single-quoted strings,
// double-quoted identifiers, and -- and /* */ comments. It does not understand
// dollar quoting or backslash escapes, so unusual SQL may be rejected. It is a
// convenience for early feedback, not a security boundary: the runner executes
// checks with the extended query protocol (one statement only) inside a
// read-only transaction on the restored copy.
func ValidateCheckSQL(sql string) error {
	if strings.TrimSpace(sql) == "" {
		return fmt.Errorf("sql must not be empty")
	}
	if len(sql) > MaxCheckSQLLength {
		return fmt.Errorf("sql must be at most %d characters", MaxCheckSQLLength)
	}
	errMulti := fmt.Errorf("sql must be a single statement")
	ended := false        // a ';' terminator has been seen
	hasStatement := false // some statement text has been seen
	for i := 0; i < len(sql); i++ {
		c := sql[i]
		switch {
		case c == '\'' || c == '"':
			if ended {
				return errMulti
			}
			hasStatement = true
			end := closingQuote(sql, i)
			if end < 0 {
				return fmt.Errorf("sql has an unterminated quote")
			}
			i = end
		case c == '-' && i+1 < len(sql) && sql[i+1] == '-':
			nl := strings.IndexByte(sql[i:], '\n')
			if nl < 0 {
				i = len(sql)
			} else {
				i += nl
			}
		case c == '/' && i+1 < len(sql) && sql[i+1] == '*':
			closeAt := strings.Index(sql[i+2:], "*/")
			if closeAt < 0 {
				return fmt.Errorf("sql has an unterminated comment")
			}
			i += 2 + closeAt + 1
		case c == ';':
			if !hasStatement {
				return fmt.Errorf("sql contains no statement before ';'")
			}
			ended = true
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			// whitespace
		default:
			if ended {
				return errMulti
			}
			hasStatement = true
		}
	}
	if !hasStatement {
		return fmt.Errorf("sql contains no statement")
	}
	return nil
}

// closingQuote returns the index of the quote that closes the one at s[start],
// treating a doubled quote as an escaped quote, or -1 if there is none.
func closingQuote(s string, start int) int {
	q := s[start]
	for j := start + 1; j < len(s); j++ {
		if s[j] != q {
			continue
		}
		if j+1 < len(s) && s[j+1] == q {
			j++
			continue
		}
		return j
	}
	return -1
}

// ValidateTimeZone checks that name is an IANA time zone name.
// "Local" and the empty string are rejected because they depend on the host.
func ValidateTimeZone(name string) error {
	if name == "" || strings.EqualFold(name, "local") {
		return fmt.Errorf("time zone must be an IANA name such as UTC or Asia/Kolkata")
	}
	if _, err := time.LoadLocation(name); err != nil {
		return fmt.Errorf("unknown time zone %q: %w", name, err)
	}
	return nil
}
