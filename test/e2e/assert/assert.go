//go:build e2e

package assert

import (
	"os"
	"testing"
)

func DirExists(t *testing.T, files []os.FileInfo, dirName string) {
	dirFound := false
	for _, file := range files {
		if file.IsDir() && file.Name() == dirName {
			dirFound = true
			break
		}
	}

	if !dirFound {
		t.Errorf("Expected directory '%s' not found in file list", dirName)
	}
}

func DirNotExists(t *testing.T, files []os.FileInfo, dirName string) {
	dirFound := false
	for _, file := range files {
		if file.IsDir() && file.Name() == dirName {
			dirFound = true
			break
		}
	}

	if dirFound {
		t.Errorf("Directory '%s' found, but should not exist", dirName)
	}
}
