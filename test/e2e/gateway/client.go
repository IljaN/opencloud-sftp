//go:build e2e

package gateway

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"github.com/IljaN/opencloud-sftp/pkg/config"
	"github.com/IljaN/opencloud-sftp/pkg/vfs/spacelookup"
	"github.com/IljaN/opencloud-sftp/test/e2e/sftp"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/opencloud-eu/opencloud/pkg/registry"
	"github.com/opencloud-eu/opencloud/pkg/shared"
	ctxpkg "github.com/opencloud-eu/reva/v2/pkg/ctx"
	"github.com/opencloud-eu/reva/v2/pkg/rgrpc/todo/pool"
	"github.com/opencloud-eu/reva/v2/pkg/storagespace"
	"google.golang.org/grpc/metadata"
	"io"
	"net/http"
	"path"
	"strconv"
	"time"
)

// Config represents the configuration for integration tests
type Config struct {
	GatewayAddress    string `env:"GATEWAY_ADDRESS" default:"eu.opencloud.api.gateway"`
	MachineAuthAPIKey string `env:"MACHINE_AUTH_API_KEY" default:"change-me-please"`
	Insecure          bool   `env:"INSECURE" default:"true"`
}

// NewTestConfig creates a new test configuration with default values
func NewTestConfig() *Config {
	return &Config{
		GatewayAddress:    "eu.opencloud.api.gateway",
		MachineAuthAPIKey: "3RIWu=LE5kNUqDHM0xH*Dxe=U9sczrwY",
		Insecure:          true,
	}
}

// ToConfig converts Config to the main Config struct used by the SFTP server
func (tc *Config) ToConfig() *config.Config {
	return &config.Config{
		MachineAuthAPIKey: tc.MachineAuthAPIKey,
		Insecure:          tc.Insecure,
		Reva: &shared.Reva{
			Address: tc.GatewayAddress,
		},
	}
}

// Client represents a client for interacting with the gateway service
type Client struct {
	gwSelector *pool.Selector[gateway.GatewayAPIClient]
	config     *Config
	ctx        context.Context
	token      string
	user       *userpb.User
}

// NewClient creates a new gateway client for integration tests
func NewClient(config *Config, user string) (*Client, error) {
	if config == nil {
		config = NewTestConfig()
	}

	// Create the gateway selector
	cfg := config.ToConfig()
	sel, err := pool.GatewaySelector(
		cfg.Reva.Address,
		append(
			cfg.Reva.GetRevaOptions(),
			pool.WithRegistry(registry.GetRegistry()),
		)...)
	if err != nil {
		return nil, fmt.Errorf("failed to create gateway selector: %w", err)
	}

	client := &Client{
		gwSelector: sel,
		config:     config,
	}

	// Authenticate with machine auth and impersonation
	if err := client.authenticate(user); err != nil {
		return nil, fmt.Errorf("authentication failed: %w", err)
	}

	return client, nil
}

// authenticate performs machine authentication with impersonation
func (c *Client) authenticate(user string) error {
	gw, err := c.gwSelector.Next()
	if err != nil {
		return fmt.Errorf("failed to get gateway client: %w", err)
	}

	// Authenticate using machine auth with impersonation
	authRes, err := gw.Authenticate(context.Background(), &gateway.AuthenticateRequest{
		Type:         "machine",
		ClientId:     "username:" + user,
		ClientSecret: c.config.MachineAuthAPIKey,
	})

	if err != nil {
		return fmt.Errorf("authentication request failed: %w", err)
	}

	if authRes.Status.Code != rpc.Code_CODE_OK {
		return fmt.Errorf("authentication failed with status: %s", authRes.Status.Message)
	}

	// Set up authenticated context
	c.user = authRes.GetUser()
	c.token = authRes.GetToken()
	c.ctx = ctxpkg.ContextSetUser(context.Background(), c.user)
	c.ctx = metadata.AppendToOutgoingContext(c.ctx, ctxpkg.TokenHeader, c.token)
	c.ctx = ctxpkg.ContextSetToken(c.ctx, c.token)

	return nil
}

func (c *Client) TouchFile(absolutePath string) error {
	gw, err := c.gwSelector.Next()
	if err != nil {
		return fmt.Errorf("failed to get gateway client: %w", err)
	}

	spacesRes, err := gw.ListStorageSpaces(c.ctx, &provider.ListStorageSpacesRequest{})
	if err != nil {
		return fmt.Errorf("failed to list storage spaces: %w", err)
	}

	if spacesRes.Status.Code != rpc.Code_CODE_OK {
		return fmt.Errorf("list storage spaces failed with status: %s", spacesRes.Status.Message)
	}

	targetSpace, relPath, err := spacelookup.FindSpaceForPath(absolutePath, spacesRes.GetStorageSpaces())
	if err != nil {
		return fmt.Errorf("failed to find storage space for path %s: %w", absolutePath, err)
	}

	if targetSpace == nil {
		return fmt.Errorf("no suitable storage space found")
	}

	resourceId, err := storagespace.ParseID(targetSpace.GetId().GetOpaqueId())
	if err != nil {
		return fmt.Errorf("failed to parse storage space ID: %w", err)
	}

	// Create the folder reference
	folderRef := &provider.Reference{
		ResourceId: &resourceId,
		Path:       relPath,
	}

	touchRes, err := gw.TouchFile(c.ctx, &provider.TouchFileRequest{
		Ref: folderRef,
	})

	if err != nil {
		return fmt.Errorf("touch file failed: %w", err)
	}

	if touchRes.Status.Code != rpc.Code_CODE_OK {
		return fmt.Errorf("touch file failed with status: %s", touchRes.Status.Message)
	}

	return nil
}

func (c *Client) CreateFile(absolutePath string, content []byte) error {
	gw, err := c.gwSelector.Next()
	if err != nil {
		return fmt.Errorf("failed to get gateway client: %w", err)
	}

	spacesRes, err := gw.ListStorageSpaces(c.ctx, &provider.ListStorageSpacesRequest{})
	if err != nil {
		return fmt.Errorf("failed to list storage spaces: %w", err)
	}

	if spacesRes.Status.Code != rpc.Code_CODE_OK {
		return fmt.Errorf("list storage spaces failed with status: %s", spacesRes.Status.Message)
	}

	targetSpace, relPath, err := spacelookup.FindSpaceForPath(absolutePath, spacesRes.GetStorageSpaces())
	if err != nil {
		return fmt.Errorf("failed to find storage space for path %s: %w", absolutePath, err)
	}

	if targetSpace == nil {
		return fmt.Errorf("no suitable storage space found")
	}

	resourceId, err := storagespace.ParseID(targetSpace.GetId().GetOpaqueId())
	if err != nil {
		return fmt.Errorf("failed to parse storage space ID: %w", err)
	}

	// Create the folder reference
	fileRef := &provider.Reference{
		ResourceId: &resourceId,
		Path:       relPath,
	}

	// Prepare upload request
	opaque := &types.Opaque{
		Map: map[string]*types.OpaqueEntry{
			"Upload-Length": {
				Decoder: "plain",
				Value:   []byte(strconv.FormatInt(int64(len(content)), 10)),
			},
		},
	}

	uploadReq := &provider.InitiateFileUploadRequest{
		Ref:    fileRef,
		Opaque: opaque,
	}

	touchRes, err := gw.InitiateFileUpload(c.ctx, uploadReq)

	if err != nil {
		return fmt.Errorf("touch file failed: %w", err)
	}

	if touchRes.Status.Code != rpc.Code_CODE_OK {
		return fmt.Errorf("touch file failed with status: %s", touchRes.Status.Message)
	}

	// Find the simple/spaces protocol endpoint
	var uploadEndpoint, uploadToken string
	for _, proto := range touchRes.GetProtocols() {
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
	httpReq, err := http.NewRequest("PUT", uploadEndpoint, bytes.NewReader(content))
	if err != nil {
		return err
	}

	httpReq.ContentLength = int64(len(content))

	// Add auth token from context
	if token, err := extractAuthToken(c.ctx); err == nil {
		httpReq.Header.Add("X-Access-Token", token)
	}

	if uploadToken != "" {
		httpReq.Header.Add("X-Reva-Transfer", uploadToken)
	}

	hclient := http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				MinVersion:         tls.VersionTLS12,
				InsecureSkipVerify: true, // TODO: make configurable
			},
		},
		Timeout: 30 * time.Second,
	}
	httpResp, err := hclient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("upload request failed: %w", err)
	}

	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(httpResp.Body)
		return fmt.Errorf("upload failed with status %d: %s", httpResp.StatusCode, string(body))
	}

	return nil
}

func extractAuthToken(ctx context.Context) (string, error) {
	md, ok := metadata.FromOutgoingContext(ctx)
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

func (c *Client) CreateFolder(absolutePath string) error {
	gw, err := c.gwSelector.Next()
	if err != nil {
		return fmt.Errorf("failed to get gateway client: %w", err)
	}

	// Get storage spaces to find the target space
	spacesRes, err := gw.ListStorageSpaces(c.ctx, &provider.ListStorageSpacesRequest{})
	if err != nil {
		return fmt.Errorf("failed to list storage spaces: %w", err)
	}

	if spacesRes.Status.Code != rpc.Code_CODE_OK {
		return fmt.Errorf("list storage spaces failed with status: %s", spacesRes.Status.Message)
	}

	targetSpace, relPath, err := spacelookup.FindSpaceForPath(absolutePath, spacesRes.GetStorageSpaces())
	if err != nil {
		return fmt.Errorf("failed to find storage space for path %s: %w", absolutePath, err)
	}

	if targetSpace == nil {
		return fmt.Errorf("no suitable storage space found")
	}

	resourceId, err := storagespace.ParseID(targetSpace.GetId().GetOpaqueId())
	if err != nil {
		return fmt.Errorf("failed to parse storage space ID: %w", err)
	}

	// Create the folder reference
	folderRef := &provider.Reference{
		ResourceId: &resourceId,
		Path:       relPath,
	}

	// Create the folder
	createRes, err := gw.CreateContainer(c.ctx, &provider.CreateContainerRequest{
		Ref: folderRef,
	})

	if err != nil {
		return fmt.Errorf("create container request failed: %w", err)
	}

	if createRes.Status.Code != rpc.Code_CODE_OK {
		return fmt.Errorf("create container failed with status: %s", createRes.Status.Message)
	}

	return nil
}

// ListFolder lists the contents of a folder
func (c *Client) ListFolder(absolutePath string) ([]*provider.ResourceInfo, error) {
	gw, err := c.gwSelector.Next()
	if err != nil {
		return nil, fmt.Errorf("failed to get gateway client: %w", err)
	}

	// Get storage spaces to find the target space
	spacesRes, err := gw.ListStorageSpaces(c.ctx, &provider.ListStorageSpacesRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to list storage spaces: %w", err)
	}

	if spacesRes.Status.Code != rpc.Code_CODE_OK {
		return nil, fmt.Errorf("list storage spaces failed with status: %s", spacesRes.Status.Message)
	}

	targetSpace, relPath, err := spacelookup.FindSpaceForPath(absolutePath, spacesRes.GetStorageSpaces())
	if err != nil {
		return nil, fmt.Errorf("failed to find storage space for path %s: %w", absolutePath, err)
	}

	if targetSpace == nil {
		return nil, fmt.Errorf("no suitable storage space found")
	}

	resourceId, err := storagespace.ParseID(targetSpace.GetId().GetOpaqueId())
	if err != nil {
		return nil, fmt.Errorf("failed to parse storage space ID: %w", err)
	}

	folderRef := &provider.Reference{
		ResourceId: &resourceId,
		Path:       relPath,
	}

	listRes, err := gw.ListContainer(c.ctx, &provider.ListContainerRequest{
		Ref: folderRef,
	})

	if err != nil {
		return nil, fmt.Errorf("list container request failed: %w", err)
	}

	if listRes.Status.Code != rpc.Code_CODE_OK {
		return nil, fmt.Errorf("create container failed with status: %s", listRes.Status.Message)
	}

	return listRes.GetInfos(), nil
}

// Delete deletes a folder and its contents
func (c *Client) Delete(absolutePath string) error {
	gw, err := c.gwSelector.Next()
	if err != nil {
		return fmt.Errorf("failed to get gateway client: %w", err)
	}

	// Get storage spaces to find the target space
	spacesRes, err := gw.ListStorageSpaces(c.ctx, &provider.ListStorageSpacesRequest{})
	if err != nil {
		return fmt.Errorf("failed to list storage spaces: %w", err)
	}

	if spacesRes.Status.Code != rpc.Code_CODE_OK {
		return fmt.Errorf("list storage spaces failed with status: %s", spacesRes.Status.Message)
	}

	targetSpace, relPath, err := spacelookup.FindSpaceForPath(absolutePath, spacesRes.GetStorageSpaces())
	if err != nil {
		return fmt.Errorf("failed to find storage space for path %s: %w", absolutePath, err)
	}

	resourceId, err := storagespace.ParseID(targetSpace.GetId().GetOpaqueId())
	if err != nil {
		return fmt.Errorf("failed to parse storage space ID: %w", err)
	}

	// Create the folder reference
	pRef := &provider.Reference{
		ResourceId: &resourceId,
		Path:       relPath,
	}

	// Delete the element
	deleteRes, err := gw.Delete(c.ctx, &provider.DeleteRequest{
		Ref: pRef,
	})

	if err != nil {
		return fmt.Errorf("delete failed: %w", err)
	}

	if deleteRes.Status.Code != rpc.Code_CODE_OK {
		return fmt.Errorf("delete failed with status: %s", deleteRes.Status.Message)
	}

	return nil
}

func (c *Client) Stat(absolutePath string) (*provider.ResourceInfo, error) {
	gw, err := c.gwSelector.Next()
	if err != nil {
		return nil, fmt.Errorf("failed to get gateway client: %w", err)
	}

	spacesRes, err := gw.ListStorageSpaces(c.ctx, &provider.ListStorageSpacesRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to list storage spaces: %w", err)
	}

	if spacesRes.Status.Code != rpc.Code_CODE_OK {
		return nil, fmt.Errorf("list storage spaces failed with status: %s", spacesRes.Status.Message)
	}

	targetSpace, relPath, err := spacelookup.FindSpaceForPath(absolutePath, spacesRes.GetStorageSpaces())
	if err != nil {
		return nil, fmt.Errorf("failed to find storage space for path %s: %w", absolutePath, err)
	}

	if targetSpace == nil {
		return nil, fmt.Errorf("no suitable storage space found")
	}

	resourceId, err := storagespace.ParseID(targetSpace.GetId().GetOpaqueId())
	if err != nil {
		return nil, fmt.Errorf("failed to parse storage space ID: %w", err)
	}

	fileRef := &provider.Reference{
		ResourceId: &resourceId,
		Path:       relPath,
	}

	statRes, err := gw.Stat(c.ctx, &provider.StatRequest{
		Ref: fileRef,
	})

	if err != nil {
		return nil, fmt.Errorf("stat request failed: %w", err)
	}
	if statRes.Status.Code != rpc.Code_CODE_OK {
		return nil, fmt.Errorf("stat failed with status: %s", statRes.Status.Message)
	}

	return statRes.GetInfo(), nil

}

func (c *Client) DeployPublicKey(kp *sftp.KeyPair) error {
	gw, err := c.gwSelector.Next()
	if err != nil {
		return fmt.Errorf("failed to get gateway client: %w", err)
	}

	// Get storage spaces to find the target space
	spacesRes, err := gw.ListStorageSpaces(c.ctx, spacelookup.FilterForPersonalSpace(c.GetUser().GetId()))
	if err != nil {
		return fmt.Errorf("failed to list storage spaces: %w", err)
	}

	if spacesRes.Status.Code != rpc.Code_CODE_OK {
		return fmt.Errorf("list storage spaces failed with status: %s", spacesRes.Status.Message)
	}

	sspaces := spacesRes.GetStorageSpaces()

	if len(sspaces) != 1 {
		return fmt.Errorf("expected exactly one personal storage space, got %d", len(sspaces))
	}

	targetSpace := sspaces[0]
	pathRoot := targetSpace.GetName()

	remoteKeyPath := path.Join(pathRoot, ".ssh")

	_ = c.Delete(remoteKeyPath)

	err = c.CreateFolder(remoteKeyPath)
	if err != nil {
		return fmt.Errorf("failed to create .ssh folder: %w", err)
	}

	pubKeyPath := path.Join(remoteKeyPath, "ed25519.pub")
	err = c.CreateFile(pubKeyPath, kp.PublicKey)
	if err != nil {
		return fmt.Errorf("failed to upload public key file: %w", err)
	}

	return nil
}

func (c *Client) ClearPersonalSpace() error {
	gw, err := c.gwSelector.Next()
	if err != nil {
		return fmt.Errorf("failed to get gateway client: %w", err)
	}

	// Get storage spaces to find the target space
	spacesRes, err := gw.ListStorageSpaces(c.ctx, spacelookup.FilterForPersonalSpace(c.GetUser().GetId()))
	if err != nil {
		return fmt.Errorf("failed to list storage spaces: %w", err)
	}

	if spacesRes.Status.Code != rpc.Code_CODE_OK {
		return fmt.Errorf("list storage spaces failed with status: %s", spacesRes.Status.Message)
	}

	sspaces := spacesRes.GetStorageSpaces()

	if len(sspaces) != 1 {
		return fmt.Errorf("expected exactly one personal storage space, got %d", len(sspaces))
	}

	targetSpace := sspaces[0]
	pathRoot := targetSpace.GetName()

	files, err := c.ListFolder(pathRoot)
	if err != nil {
		return fmt.Errorf("failed to list personal space contents: %w", err)
	}

	for _, file := range files {
		if file.GetName() == ".ssh" {
			continue
		}
		absPath := path.Join(pathRoot, file.GetName())
		err = c.Delete(absPath)
		if err != nil {
			return fmt.Errorf("failed to delete file %s: %w", file.GetPath(), err)
		}
	}

	return nil
}

// CreateHome creates a new home for the authenticated user
func (c *Client) CreateHome() error {
	gw, err := c.gwSelector.Next()
	if err != nil {
		return fmt.Errorf("failed to get gateway client: %w", err)
	}

	res, err := gw.CreateHome(c.ctx, &provider.CreateHomeRequest{})
	if err != nil {
		return fmt.Errorf("failed to create home: %w", err)
	}

	if res.Status.Code != rpc.Code_CODE_OK {
		return fmt.Errorf("create home failed with status: %s", res.Status.Message)
	}

	return nil
}

// GetUser returns the authenticated user
func (c *Client) GetUser() *userpb.User {
	return c.user
}

// GetToken returns the authentication token
func (c *Client) GetToken() string {
	return c.token
}

// Close closes the client connection
func (c *Client) Close() error {
	// The pool selector doesn't need explicit closing
	return nil
}
