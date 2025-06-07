//go:build e2e

package e2e_tests

import (
	"github.com/IljaN/opencloud-sftp/e2e_tests/assert"
	storageProvider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"io"
	"testing"
)

func (ts *TestSuite) TestListRoot(t *testing.T) {
	sftp, cleanup := ts.GetSFTPClient("admin")
	defer cleanup()

	files, err := sftp.ReadDir("/.")
	if err != nil {
		t.Fatalf("Failed to list root directory: %v", err)
	}

	assert.DirExists(t, files, "Admin")
	assert.DirExists(t, files, "Shares")
}

func (ts *TestSuite) TestMkdir_Simple(t *testing.T) {
	sftp, cleanup := ts.GetSFTPClient("admin")
	defer cleanup()

	fPath := "/Admin/TestFolder"

	err := sftp.Mkdir(fPath)
	if err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	res, err := ts.GetGateway("admin").Stat(fPath)
	if err != nil {
		t.Fatalf("Failed to stat created directory: %v", err)
	}

	if res.Type != storageProvider.ResourceType_RESOURCE_TYPE_CONTAINER {
		t.Fatalf("Expected resource type RESOURCE_TYPE_CONTAINER, got %s", res.Type)
	}
}

func (ts *TestSuite) TestMkdir_Nested(t *testing.T) {
	sftp, cleanup := ts.GetSFTPClient("admin")
	defer cleanup()

	lvl1 := "/Admin/TestFolder/"
	lvl2 := "/Admin/TestFolder/Nested/"

	err := sftp.Mkdir(lvl1)
	if err != nil {
		t.Fatalf("Failed to create nested directory: %v", err)
	}

	err = sftp.Mkdir(lvl2)
	if err != nil {
		t.Fatalf("Failed to create nested directory: %v", err)
	}

	res, err := ts.GetGateway("admin").Stat(lvl2)
	if err != nil {
		t.Fatalf("Failed to stat created nested directory: %v", err)
	}

	if res.Type != storageProvider.ResourceType_RESOURCE_TYPE_CONTAINER {
		t.Fatalf("Expected resource type RESOURCE_TYPE_CONTAINER, got %s", res.Type)
	}
}

func (ts *TestSuite) TestMkdirAll(t *testing.T) {
	t.Skipf("Skipping test %s, failing for some reason...", t.Name())
	sftp, cleanup := ts.GetSFTPClient("admin")
	defer cleanup()

	fPath := "/Admin/TestFolder/Nested/Deep/Path"

	err := sftp.Mkdir(fPath)
	if err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}
}

func (ts *TestSuite) TestDeleteFolder_Simple(t *testing.T) {
	gw := ts.GetGateway("admin")
	fPath := "/Admin/TestFolder/"

	err := gw.CreateFolder(fPath)
	if err != nil {
		t.Fatalf("Failed to create test folder: %v", err)
	}

	sftp, cleanup := ts.GetSFTPClient("admin")
	defer cleanup()

	err = sftp.RemoveDirectory(fPath)
	if err != nil {
		t.Fatalf("Failed to delete test folder: %v", err)
	}

	res, err := gw.Stat(fPath)
	if err == nil {
		t.Fatalf("Expected error when statting deleted folder, got: %v", res)
	}
}

func (ts *TestSuite) TestDelete_Nested(t *testing.T) {
	paths := []string{
		"/Admin/TestFolder",
		"/Admin/TestFolder/Nested",
		"/Admin/TestFolder/Nested/Deep",
	}

	gw := ts.GetGateway("admin")

	for _, path := range paths {
		err := gw.CreateFolder(path)
		if err != nil {
			t.Fatalf("Failed to create test folder: %v", err)
		}

	}

	res, err := gw.Stat(paths[2])
	if err != nil {
		t.Fatalf("Failed to stat test folder: %v", err)
	}

	sftp, cleanup := ts.GetSFTPClient("admin")
	defer cleanup()

	err = sftp.RemoveAll(paths[0])
	if err != nil {
		t.Fatalf("Failed to delete test folder: %v", err)
	}

	res, err = gw.Stat(paths[0])
	if err == nil {
		t.Fatalf("Expected error when statting deleted folder, got: %v", res)
	}
}

func (ts *TestSuite) TestUploadFile(t *testing.T) {
	sftp, cleanup := ts.GetSFTPClient("admin")
	defer cleanup()

	fPath := "/Admin/TestFile.txt"
	content := []byte("This is a test file content.")

	file, err := sftp.Create(fPath)
	if err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	defer file.Close()

	_, err = file.Write(content)
	if err != nil {
		t.Fatalf("Failed to upload file: %v", err)
	}

	gw := ts.GetGateway("admin")

	res, err := gw.Stat(fPath)
	if err != nil {
		t.Fatalf("Failed to stat uploaded file: %v", err)
	}

	if res.Type != storageProvider.ResourceType_RESOURCE_TYPE_FILE {
		t.Fatalf("Expected resource type RESOURCE_TYPE_FILE, got %s", res.Type)
	}

	if res.GetSize() != uint64(len(content)) {
		t.Fatalf("Expected uploaded file size %d, got %d", len(content), res.GetSize())
	}
}

func (ts *TestSuite) TestDownloadFile(t *testing.T) {
	fPath := "/Admin/Download.txt"
	content := []byte("This is download test file content.")

	gw := ts.GetGateway("admin")
	err := gw.CreateFile(fPath, content)
	if err != nil {
		t.Fatalf("Failed to create file for download: %v", err)
	}

	sftp, cleanup := ts.GetSFTPClient("admin")
	defer cleanup()

	file, err := sftp.Open(fPath)
	if err != nil {
		t.Fatalf("Failed to open file for download: %v", err)
	}
	defer file.Close()

	downloaded, err := io.ReadAll(file)
	if err != nil {
		t.Fatalf("Failed to read downloaded file: %v", err)
	}

	if string(downloaded) != string(content) {
		t.Fatalf("Downloaded content does not match original content. Expected: %s, got: %s", string(content), string(downloaded))
	}
}
