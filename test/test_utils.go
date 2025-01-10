// Copyright Red Hat, Inc.
//
// Licensed under the Apache License, Version 2.0 (the License);
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an AS IS BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var rootDir = ""

// ProjectRoot finds root of the project based on the assumption that it is
// the first parent directory with `go.mod` file.
func ProjectRoot() string {
	if rootDir != "" {
		// Project root directory has already been resolved, return what's found.
		return rootDir
	}

	currentDir, err := os.Getwd()
	if err != nil {
		panic(fmt.Sprintf("failed to get current working directory: %v", err))
	}

	for {
		if _, err := os.Stat(filepath.Join(currentDir, "go.mod")); err == nil {
			rootDir = filepath.FromSlash(currentDir)
			break
		}

		parentDir := filepath.Dir(currentDir)
		if parentDir == currentDir {
			break
		}

		currentDir = parentDir
	}

	if rootDir == "" {
		panic(fmt.Sprintf("failed to get current working directory: %v", err))
	}

	return rootDir
}

func Indent(spaces int, text string) string {
	prefix := strings.Repeat(" ", spaces)
	return prefix + strings.ReplaceAll(text, "\n", "\n"+prefix)
}
