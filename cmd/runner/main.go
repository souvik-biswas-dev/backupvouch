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

package main

import (
	"context"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: runner <backup|verify|prune>\n")
		os.Exit(1)
	}

	ctx := context.Background()
	subcommand := os.Args[1]

	switch subcommand {
	case "backup":
		if err := RunBackup(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "backup failed: %v\n", err)
			os.Exit(1)
		}
	case "verify":
		if err := RunVerify(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "verify failed: %v\n", err)
			os.Exit(1)
		}
	case "prune":
		fmt.Fprintf(os.Stderr, "prune subcommand will be implemented in Step 4/5\n")
		os.Exit(1)
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q. Expected: backup, verify, or prune\n", subcommand)
		os.Exit(1)
	}
}
