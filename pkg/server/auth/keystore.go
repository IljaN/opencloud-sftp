package auth

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	sftpSvrCfg "github.com/IljaN/opencloud-sftp/pkg/config"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	providerv1beta1 "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/gliderlabs/ssh"
	"github.com/opencloud-eu/opencloud/pkg/log"
	ctxpkg "github.com/opencloud-eu/reva/v2/pkg/ctx"
	"github.com/opencloud-eu/reva/v2/pkg/rgrpc/todo/pool"
	"github.com/opencloud-eu/reva/v2/pkg/storagespace"
	"github.com/opencloud-eu/reva/v2/pkg/utils"
	gossh "golang.org/x/crypto/ssh"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

type PubKeyStorage interface {
	LoadKeys(ctx context.Context, userName string) ([]ssh.PublicKey, error)
}

func NewSpaceKeyStorage(cfg *sftpSvrCfg.Config, gwSelector *pool.Selector[gateway.GatewayAPIClient], logger log.Logger) PubKeyStorage {
	return &SpaceKeyStorage{
		cfg:        cfg,
		gwSelector: gwSelector,
		log:        logger,
		cl: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					MinVersion:         tls.VersionTLS12,
					InsecureSkipVerify: true,
				},
			},
			Timeout: 30 * time.Second,
		},
	}
}

// SpaceKeyStorage loads public keys from the ".ssh" folder in the root of the personal space of the user
type SpaceKeyStorage struct {
	cfg        *sftpSvrCfg.Config
	gwSelector *pool.Selector[gateway.GatewayAPIClient]
	log        log.Logger
	cl         *http.Client
}

func (p *SpaceKeyStorage) LoadKeys(ctx context.Context, userName string) ([]ssh.PublicKey, error) {
	// Get user's personal storage space
	gwapi, err := p.gwSelector.Next()
	if err != nil {
		return nil, fmt.Errorf("failed to get gateway client: %w", err)
	}

	user, ok := ctxpkg.ContextGetUser(ctx)
	if !ok {
		return nil, fmt.Errorf("failed to get user from context")
	}

	lSSRes, err := gwapi.ListStorageSpaces(ctx, &providerv1beta1.ListStorageSpacesRequest{
		FieldMask: &fieldmaskpb.FieldMask{Paths: []string{"*"}},
		Filters: []*providerv1beta1.ListStorageSpacesRequest_Filter{
			{
				Type: providerv1beta1.ListStorageSpacesRequest_Filter_TYPE_SPACE_TYPE,
				Term: &providerv1beta1.ListStorageSpacesRequest_Filter_SpaceType{
					SpaceType: "personal",
				},
			},
			{
				Type: providerv1beta1.ListStorageSpacesRequest_Filter_TYPE_OWNER,
				Term: &providerv1beta1.ListStorageSpacesRequest_Filter_Owner{
					Owner: user.GetId(),
				},
			},
		},
	})
	if err != nil || lSSRes.Status.Code != rpc.Code_CODE_OK {
		return nil, fmt.Errorf("failed to list storage spaces for user %s: %v", userName, err)
	}

	storageSpaces := lSSRes.GetStorageSpaces()
	if len(storageSpaces) != 1 {
		return nil, fmt.Errorf("expected exactly one personal storage space for user %s, got %d", userName, len(storageSpaces))
	}

	primarySpace := storageSpaces[0]
	resourceID, err := storagespace.ParseID(primarySpace.GetId().GetOpaqueId())
	if err != nil {
		return nil, fmt.Errorf("failed to parse storage space ID: %w", err)
	}

	// List files in the .ssh directory
	gwapi, err = p.gwSelector.Next()
	if err != nil {
		return nil, fmt.Errorf("failed to get gateway client: %w", err)
	}

	hr, err := gwapi.ListContainer(ctx, &providerv1beta1.ListContainerRequest{
		Ref: &providerv1beta1.Reference{
			ResourceId: &resourceID,
			Path:       utils.MakeRelativePath("/.ssh"),
		},
	})
	if err != nil || hr.Status.Code != rpc.Code_CODE_OK {
		return nil, fmt.Errorf("failed to list .ssh directory for user %s: %v", userName, err)
	}

	fileInfos := hr.GetInfos()
	var publicKeys []ssh.PublicKey

	// Process each file in the .ssh directory
	for _, info := range fileInfos {
		// Skip directories and non-public key files
		if info.GetType() != providerv1beta1.ResourceType_RESOURCE_TYPE_FILE {
			continue
		}

		fileName := info.GetPath()
		// Check if this is a public key file (ends with .pub)
		if !strings.HasSuffix(fileName, ".pub") {
			continue
		}

		p.log.Debug().Str("file", fileName).Str("user", userName).Msg("Processing SSH public key file")

		token, ok := ctxpkg.ContextGetToken(ctx)
		if !ok {
			return nil, fmt.Errorf("failed to get token from context")
		}

		// Download the public key file
		pubKey, err := p.downloadPublicKey(ctx, resourceID, fileName, token)
		if err != nil {
			p.log.Warn().Err(err).Str("file", fileName).Str("user", userName).Msg("Failed to download or parse public key file")
			continue
		}

		publicKeys = append(publicKeys, pubKey)
		p.log.Debug().Str("file", fileName).Str("user", userName).Msg("Successfully loaded SSH public key")
	}

	return publicKeys, nil
}

func (p *SpaceKeyStorage) downloadPublicKey(ctx context.Context, resourceID providerv1beta1.ResourceId, fileName string, token string) (ssh.PublicKey, error) {
	gwapi, err := p.gwSelector.Next()
	if err != nil {
		return nil, fmt.Errorf("failed to get gateway client: %w", err)
	}

	// Initiate file download
	fdres, err := gwapi.InitiateFileDownload(ctx, &providerv1beta1.InitiateFileDownloadRequest{
		Opaque: nil,
		Ref: &providerv1beta1.Reference{
			ResourceId: &resourceID,
			Path:       utils.MakeRelativePath(filepath.Join("/.ssh", fileName)),
		},
	})

	if err != nil || fdres.Status.Code != rpc.Code_CODE_OK {
		return nil, fmt.Errorf("failed to initiate download for %s: %v", fileName, err)
	}

	// Find the simple/spaces protocol endpoint
	var downloadEndpoint, downloadToken string
	for _, proto := range fdres.GetProtocols() {
		if proto.GetProtocol() == "simple" || proto.GetProtocol() == "spaces" {
			downloadEndpoint = proto.GetDownloadEndpoint()
			downloadToken = proto.GetToken()
			break
		}
	}

	if downloadEndpoint == "" {
		return nil, fmt.Errorf("no suitable download protocol found for %s", fileName)
	}

	// Create HTTP request
	httpReq, err := http.NewRequest("GET", downloadEndpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request for %s: %w", fileName, err)
	}
	httpReq.Header.Add("X-Access-Token", token)
	httpReq.Header.Add("X-Reva-Transfer", downloadToken)

	dlRes, err := p.cl.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to download %s: %w", fileName, err)
	}
	defer dlRes.Body.Close()

	if dlRes.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download failed for %s with status %d", fileName, dlRes.StatusCode)
	}

	rawPubKey, err := io.ReadAll(dlRes.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s content: %w", fileName, err)
	}

	foundPubKey, _, _, _, err := gossh.ParseAuthorizedKey(rawPubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to parse public key from %s: %w", fileName, err)
	}

	return foundPubKey, nil
}
