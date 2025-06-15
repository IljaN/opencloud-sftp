package server

import (
	"context"
	sftpSvrCfg "github.com/IljaN/opencloud-sftp/pkg/config"
	"github.com/IljaN/opencloud-sftp/pkg/server/auth"
	"github.com/IljaN/opencloud-sftp/pkg/vfs"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
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
			Addr: cfg.SFTPAddress,
		},
		cfg: cfg,
		log: logger,
	}

	s.SubsystemHandlers = map[string]ssh.SubsystemHandler{
		"sftp": s.SFTPHandler,
	}

	return s
}

// SFTPHandler handler for SFTP subsystem
func (s *SFTPServer) SFTPHandler(sess ssh.Session) {
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

	vfsLogger := s.log.With().
		Str("subsystem", "vfs").
		Str("uid", sess.User()).
		Logger()

	server := sftp.NewRequestServer(
		sess,
		vfs.OpenCloudHandler(authCtx, s.gwSelector, vfsLogger),
	)

	if err := server.Serve(); err == io.EOF {
		server.Close()
		s.log.Debug().Str("uid", sess.User()).Msg("sftp client exited session.")
	} else if err != nil {
		s.log.Debug().Str("uid", sess.User()).Err(err).Msg("sftp server completed with error")
	}
}

func (s *SFTPServer) ListenAndServe() error {
	key, err := readPrivateKeyFromFile(s.cfg.HostPrivateKeyPath)
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

	s.PublicKeyHandler = auth.NewPubKeyAuthHandler(
		auth.NewSpaceKeyStorage(s.cfg, s.gwSelector, s.log),
		s.gwSelector,
		s.cfg.MachineAuthAPIKey,
	)

	return s.Server.ListenAndServe()
}

func readPrivateKeyFromFile(certPath string) (gossh.Signer, error) {
	privateBytes, err := os.ReadFile(certPath)
	if err != nil {
		return nil, err
	}

	return gossh.ParsePrivateKey(privateBytes)
}
