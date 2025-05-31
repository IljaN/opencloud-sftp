package server

import (
	"context"
	"fmt"
	sftpSvrCfg "github.com/IljaN/opencloud-sftp/pkg/config"
	"github.com/IljaN/opencloud-sftp/pkg/vfs"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	gatewayv1beta1 "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/gliderlabs/ssh"
	"github.com/opencloud-eu/opencloud/pkg/log"
	"github.com/opencloud-eu/opencloud/pkg/registry"
	ctxpkg "github.com/opencloud-eu/reva/v2/pkg/ctx"
	"github.com/opencloud-eu/reva/v2/pkg/rgrpc/todo/pool"
	"github.com/pkg/sftp"
	gossh "golang.org/x/crypto/ssh"
	"google.golang.org/grpc/metadata"
	"io"
	"os"
)

type SFTPServer struct {
	*ssh.Server

	gwSelector *pool.Selector[gateway.GatewayAPIClient]
	cfg        *sftpSvrCfg.Config
	log        log.Logger
}

func NewSFTPServer(cfg *sftpSvrCfg.Config, logger log.Logger) *SFTPServer {
	s := &SFTPServer{
		Server: &ssh.Server{
			Addr: "127.0.0.1:2222",
		},
		cfg: cfg,
		log: logger,
	}

	s.SubsystemHandlers = map[string]ssh.SubsystemHandler{
		"sftp": s.SFTPHandler,
	}

	s.PasswordHandler = s.HandleAuth

	return s
}

// SFTPHandler handler for SFTP subsystem
func (s *SFTPServer) SFTPHandler(sess ssh.Session) {
	s.gwSelector.Next()

	uid, ok := sess.Context().Value("uid").(*userpb.UserId)
	if !ok {
		s.log.Error().Msg("Failed to get uid from ctx")
		return
	}

	token, ok := sess.Context().Value("token").(string)
	if !ok {
		s.log.Error().Msg("Failed to get token from ctx")
		return
	}

	authCtx := ctxpkg.ContextSetUser(context.Background(), &userpb.User{Id: uid})
	authCtx = metadata.AppendToOutgoingContext(authCtx, ctxpkg.TokenHeader, token)

	vfsLogger := s.log.With().Str("subsystem", "vfs").Logger()
	server := sftp.NewRequestServer(
		sess,
		vfs.OpenCloudHandler(authCtx, s.gwSelector, vfsLogger),
	)

	if err := server.Serve(); err == io.EOF {
		server.Close()
		fmt.Println("sftp client exited session.")
	} else if err != nil {
		fmt.Println("sftp server completed with error:", err)
	}
}

func (s *SFTPServer) HandleAuth(ctx ssh.Context, passwd string) bool {
	gwapi, err := s.gwSelector.Next()
	if err != nil {
		return false
	}

	userName := ctx.User()

	//ctx := context.Background()
	authRes, err := gwapi.Authenticate(ctx, &gatewayv1beta1.AuthenticateRequest{
		Type:         "machine",
		ClientId:     "username:" + userName,
		ClientSecret: s.cfg.MachineAuthAPIKey,
	})

	if err != nil {
		s.log.Debug().Err(err).Msgf("Auth failed")
		return false
	}

	if authRes.GetStatus().GetCode() == rpc.Code_CODE_OK {
		ctx.SetValue("uid", authRes.GetUser().GetId())
		ctx.SetValue("token", authRes.GetToken())
		return true
	}

	authErr := fmt.Errorf(authRes.GetStatus().GetMessage())
	s.log.Debug().Err(authErr).Msgf("Auth request returned rejected for %s", userName)
	return false
}

func (s *SFTPServer) ListenAndServe() error {
	key, err := readPrivateKeyFromFile(s.cfg.ServerCertPath)
	if err != nil {
		return err
	}

	s.AddHostKey(key)

	sel, err := pool.GatewaySelector(
		s.cfg.Reva.Address,
		append(
			s.cfg.Reva.GetRevaOptions(),
			pool.WithRegistry(registry.GetRegistry()),
		)...)
	if err != nil {
		return err
	}

	s.gwSelector = sel

	return s.Server.ListenAndServe()
}

func readPrivateKeyFromFile(certPath string) (gossh.Signer, error) {
	privateBytes, err := os.ReadFile(certPath)
	if err != nil {
		return nil, err
	}

	return gossh.ParsePrivateKey(privateBytes)
}
