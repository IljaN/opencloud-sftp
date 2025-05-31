package vfs

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"google.golang.org/grpc/metadata"
)

// sftpFileHandler implements io.ReaderAt and io.WriterAt for SFTP file operations
type sftpFileHandler struct {
	fs       *root
	ref      *provider.Reference
	filepath string
	flags    uint32

	// Caching
	mu         sync.RWMutex
	cache      []byte
	cacheValid bool
	fileSize   int64
	etag       string

	// HTTP client for data gateway operations
	httpClient *http.Client
}

// newSftpFileHandler creates a new file handler
func newSftpFileHandler(fs *root, ref *provider.Reference, filepath string, flags uint32) *sftpFileHandler {
	return &sftpFileHandler{
		fs:       fs,
		ref:      ref,
		filepath: filepath,
		flags:    flags,
		httpClient: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					MinVersion:         tls.VersionTLS12,
					InsecureSkipVerify: true, // TODO: make configurable
				},
			},
			Timeout: 30 * time.Second,
		},
	}
}

// ReadAt implements io.ReaderAt
func (h *sftpFileHandler) ReadAt(b []byte, off int64) (n int, err error) {
	h.mu.RLock()
	if h.cacheValid && h.cache != nil {
		h.mu.RUnlock()
		return h.readFromCache(b, off)
	}
	h.mu.RUnlock()

	// Need to download the file
	if err := h.downloadFile(); err != nil {
		return 0, err
	}

	return h.readFromCache(b, off)
}

// WriteAt implements io.WriterAt
func (h *sftpFileHandler) WriteAt(b []byte, off int64) (n int, err error) {
	// For writes, we need to ensure we have the current file content
	if !h.cacheValid {
		// Try to download existing file, ignore not found errors
		_ = h.downloadFile()
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	// Extend cache if necessary
	requiredSize := off + int64(len(b))
	if int64(len(h.cache)) < requiredSize {
		newCache := make([]byte, requiredSize)
		copy(newCache, h.cache)
		h.cache = newCache
		h.fileSize = requiredSize
	}

	// Write to cache
	n = copy(h.cache[off:], b)

	// Upload the modified content
	if err := h.uploadFile(); err != nil {
		return n, err
	}

	return n, nil
}

// readFromCache reads data from the cache
func (h *sftpFileHandler) readFromCache(b []byte, off int64) (n int, err error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if off >= h.fileSize {
		return 0, io.EOF
	}

	n = copy(b, h.cache[off:])
	if off+int64(n) >= h.fileSize {
		return n, io.EOF
	}

	return n, nil
}

// downloadFile downloads the file content and caches it
func (h *sftpFileHandler) downloadFile() error {
	client, err := h.fs.gwSelector.Next()
	if err != nil {
		return err
	}

	// First, stat the file to get its info
	statResp, err := client.Stat(h.fs.authCtx, &provider.StatRequest{
		Ref: h.ref,
	})
	if err != nil {
		return err
	}
	if statResp.Status.Code != rpc.Code_CODE_OK && statResp.Status.Code != rpc.Code_CODE_NOT_FOUND {
		return fmt.Errorf("stat failed: %s", statResp.Status.Message)
	}

	// If file doesn't exist, initialize empty cache
	if statResp.Status.Code == rpc.Code_CODE_NOT_FOUND {
		h.mu.Lock()
		h.cache = []byte{}
		h.cacheValid = true
		h.fileSize = 0
		h.mu.Unlock()
		return nil
	}

	// Initiate download
	resp, err := client.InitiateFileDownload(h.fs.authCtx, &provider.InitiateFileDownloadRequest{
		Ref: h.ref,
	})
	if err != nil {
		return err
	}
	if resp.Status.Code != rpc.Code_CODE_OK {
		return fmt.Errorf("initiate download failed: %s", resp.Status.Message)
	}

	// Find the simple/spaces protocol endpoint
	var downloadEndpoint, downloadToken string
	for _, proto := range resp.GetProtocols() {
		if proto.GetProtocol() == "simple" || proto.GetProtocol() == "spaces" {
			downloadEndpoint = proto.GetDownloadEndpoint()
			downloadToken = proto.GetToken()
			break
		}
	}

	if downloadEndpoint == "" {
		return fmt.Errorf("no suitable download protocol found")
	}

	// Create HTTP request
	httpReq, err := http.NewRequest("GET", downloadEndpoint, nil)
	if err != nil {
		return err
	}

	// Add auth token from context
	if token, err := h.extractAuthToken(); err == nil {
		httpReq.Header.Add("X-Access-Token", token)
	}

	if downloadToken != "" {
		httpReq.Header.Add("X-Reva-Transfer", downloadToken)
	}

	// Execute download
	httpResp, err := h.httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status: %d", httpResp.StatusCode)
	}

	// Read the content
	content, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return err
	}

	// Update cache
	h.mu.Lock()
	h.cache = content
	h.cacheValid = true
	h.fileSize = int64(len(content))
	h.etag = httpResp.Header.Get("ETag")
	h.mu.Unlock()

	h.fs.log.Debug().
		Str("path", h.filepath).
		Int64("size", h.fileSize).
		Msg("File downloaded and cached")

	return nil
}

// uploadFile uploads the cached content to storage
func (h *sftpFileHandler) uploadFile() error {
	client, err := h.fs.gwSelector.Next()
	if err != nil {
		return err
	}

	// Prepare upload request
	opaque := &types.Opaque{
		Map: map[string]*types.OpaqueEntry{
			"Upload-Length": {
				Decoder: "plain",
				Value:   []byte(strconv.FormatInt(h.fileSize, 10)),
			},
		},
	}

	uploadReq := &provider.InitiateFileUploadRequest{
		Ref:    h.ref,
		Opaque: opaque,
	}

	// Add etag for conflict detection if we have one
	if h.etag != "" {
		uploadReq.Options = &provider.InitiateFileUploadRequest_IfMatch{
			IfMatch: h.etag,
		}
	}

	resp, err := client.InitiateFileUpload(h.fs.authCtx, uploadReq)
	if err != nil {
		return err
	}
	if resp.Status.Code != rpc.Code_CODE_OK {
		return fmt.Errorf("initiate upload failed: %s", resp.Status.Message)
	}

	// Find the simple/spaces protocol endpoint
	var uploadEndpoint, uploadToken string
	for _, proto := range resp.GetProtocols() {
		if proto.GetProtocol() == "simple" || proto.GetProtocol() == "spaces" {
			uploadEndpoint = proto.GetUploadEndpoint()
			uploadToken = proto.GetToken()
			break
		}
	}

	if uploadEndpoint == "" {
		return fmt.Errorf("no suitable upload protocol found")
	}

	// Create HTTP request
	httpReq, err := http.NewRequest("PUT", uploadEndpoint, bytes.NewReader(h.cache[:h.fileSize]))
	if err != nil {
		return err
	}

	httpReq.ContentLength = h.fileSize

	// Add auth token from context
	if token, err := h.extractAuthToken(); err == nil {
		httpReq.Header.Add("X-Access-Token", token)
	}

	if uploadToken != "" {
		httpReq.Header.Add("X-Reva-Transfer", uploadToken)
	}

	// Execute upload
	httpResp, err := h.httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(httpResp.Body)
		return fmt.Errorf("upload failed with status %d: %s", httpResp.StatusCode, string(body))
	}

	// Update etag from response
	if newEtag := httpResp.Header.Get("ETag"); newEtag != "" {
		h.etag = newEtag
	}

	h.fs.log.Debug().
		Str("path", h.filepath).
		Int64("size", h.fileSize).
		Msg("File uploaded successfully")

	return nil
}

// extractAuthToken extracts the auth token from the context
func (h *sftpFileHandler) extractAuthToken() (string, error) {
	md, ok := metadata.FromOutgoingContext(h.fs.authCtx)
	if !ok {
		return "", fmt.Errorf("no metadata in context")
	}

	tokens := md.Get("x-access-token")
	if len(tokens) == 0 {
		// Try alternative header names
		tokens = md.Get("authorization")
		if len(tokens) == 0 {
			return "", fmt.Errorf("no auth token found")
		}
	}

	return tokens[0], nil
}

// Truncate implements file truncation
func (h *sftpFileHandler) Truncate(size int64) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if size < 0 {
		return fmt.Errorf("negative size not allowed")
	}

	// Ensure we have the current content
	if !h.cacheValid {
		h.mu.Unlock()
		if err := h.downloadFile(); err != nil {
			return err
		}
		h.mu.Lock()
	}

	// Resize cache
	if size == 0 {
		h.cache = []byte{}
	} else if size < int64(len(h.cache)) {
		h.cache = h.cache[:size]
	} else {
		// Extend with zeros
		newCache := make([]byte, size)
		copy(newCache, h.cache)
		h.cache = newCache
	}

	h.fileSize = size

	// Upload the truncated file
	return h.uploadFile()
}
