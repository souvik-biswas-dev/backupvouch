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

package checks

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	backupv1alpha1 "github.com/souvik-biswas-dev/backupvouch/api/v1alpha1"
)

// CheckResult represents the outcome of a single SQL check.
type CheckResult struct {
	Name    string `json:"name"`
	Passed  bool   `json:"passed"`
	Message string `json:"message,omitempty"`
}

// EvaluateValue compares the value returned by a check query with the expected text.
// Normalizes booleans ("true" / "false") and prevents leaking sensitive row data in messages:
// - Expected value defaults to "true" if empty.
// - If actual value does not match:
//   - If actual value is a boolean or number (<= 20 chars), reports: expected "...", got "..."
//   - Otherwise reports: expected "...", got non-matching value
func EvaluateValue(val any, expect string) (bool, string) {
	if expect == "" {
		expect = backupv1alpha1.DefaultCheckExpect
	}

	actualText := normalizeValue(val)
	if actualText == expect {
		return true, ""
	}

	// Format mismatch message adhering to data privacy rules (docs/ARCHITECTURE.md line 119)
	if isBooleanOrShortNumber(actualText) {
		msg := fmt.Sprintf("expected %q, got %q", expect, actualText)
		return false, sanitizeMessage(msg)
	}

	msg := fmt.Sprintf("expected %q, got non-matching value", expect)
	return false, sanitizeMessage(msg)
}

func normalizeValue(val any) string {
	if val == nil {
		return "null"
	}

	switch v := val.(type) {
	case bool:
		if v {
			return "true"
		}
		return "false"
	case []byte:
		s := strings.TrimSpace(string(v))
		if s == "t" || s == "true" {
			return "true"
		}
		if s == "f" || s == "false" {
			return "false"
		}
		return s
	case string:
		s := strings.TrimSpace(v)
		if s == "t" || s == "true" {
			return "true"
		}
		if s == "f" || s == "false" {
			return "false"
		}
		return s
	case int64:
		return strconv.FormatInt(v, 10)
	case int:
		return strconv.Itoa(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", v))
	}
}

func isBooleanOrShortNumber(s string) bool {
	if s == "true" || s == "false" {
		return true
	}
	if len(s) > 20 {
		return false
	}
	// Check if integer or float
	if _, err := strconv.ParseFloat(s, 64); err == nil {
		return true
	}
	return false
}

func sanitizeMessage(msg string) string {
	msg = strings.TrimSpace(msg)
	msg = strings.ReplaceAll(msg, "\n", " ")
	msg = strings.ReplaceAll(msg, "\r", " ")
	if len(msg) > 120 {
		msg = msg[:117] + "..."
	}
	return msg
}

// RunCheck executes a single SQL check inside a read-only transaction with a statement timeout.
func RunCheck(ctx context.Context, db *sql.DB, check backupv1alpha1.Check) CheckResult {
	if err := backupv1alpha1.ValidateCheckSQL(check.SQL); err != nil {
		return CheckResult{
			Name:    check.Name,
			Passed:  false,
			Message: sanitizeMessage(err.Error()),
		}
	}

	// 15 seconds statement timeout per check
	checkCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	tx, err := db.BeginTx(checkCtx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return CheckResult{
			Name:    check.Name,
			Passed:  false,
			Message: sanitizeMessage(fmt.Sprintf("failed to begin read-only transaction: %v", err)),
		}
	}
	defer func() {
		_ = tx.Rollback()
	}()

	rows, err := tx.QueryContext(checkCtx, check.SQL)
	if err != nil {
		return CheckResult{
			Name:    check.Name,
			Passed:  false,
			Message: sanitizeMessage(fmt.Sprintf("query error: %v", err)),
		}
	}
	defer func() {
		_ = rows.Close()
	}()

	cols, err := rows.Columns()
	if err != nil {
		return CheckResult{
			Name:    check.Name,
			Passed:  false,
			Message: sanitizeMessage(fmt.Sprintf("failed to read columns: %v", err)),
		}
	}
	if len(cols) != 1 {
		return CheckResult{
			Name:    check.Name,
			Passed:  false,
			Message: "query must return exactly one column",
		}
	}

	if !rows.Next() {
		return CheckResult{
			Name:    check.Name,
			Passed:  false,
			Message: "query returned zero rows",
		}
	}

	var val any
	if err := rows.Scan(&val); err != nil {
		return CheckResult{
			Name:    check.Name,
			Passed:  false,
			Message: sanitizeMessage(fmt.Sprintf("failed to scan row: %v", err)),
		}
	}

	if rows.Next() {
		return CheckResult{
			Name:    check.Name,
			Passed:  false,
			Message: "query returned more than one row",
		}
	}

	if err := rows.Err(); err != nil {
		return CheckResult{
			Name:    check.Name,
			Passed:  false,
			Message: sanitizeMessage(fmt.Sprintf("row error: %v", err)),
		}
	}

	passed, msg := EvaluateValue(val, check.Expect)
	return CheckResult{
		Name:    check.Name,
		Passed:  passed,
		Message: msg,
	}
}

// RunChecks executes a list of SQL checks and reports the collected results.
// Returns results, whether all checks passed, and any unexpected error.
func RunChecks(ctx context.Context, db *sql.DB, checks []backupv1alpha1.Check) ([]CheckResult, bool, error) {
	if len(checks) == 0 {
		return nil, true, nil
	}

	results := make([]CheckResult, 0, len(checks))
	allPassed := true

	for _, check := range checks {
		res := RunCheck(ctx, db, check)
		results = append(results, res)
		if !res.Passed {
			allPassed = false
		}
	}

	return results, allPassed, nil
}
