package auth

import (
	"context"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/gliderlabs/ssh"
	ctxpkg "github.com/opencloud-eu/reva/v2/pkg/ctx"
	"github.com/opencloud-eu/reva/v2/pkg/rgrpc/todo/pool"
	"google.golang.org/grpc/metadata"
)

func NewPubKeyAuthHandler(ks PubKeyStorage, gwSelector *pool.Selector[gateway.GatewayAPIClient], machineAuthAPIKey string) ssh.PublicKeyHandler {
	h := pubKeyAuthHandler{
		ks:     ks,
		gw:     gwSelector,
		apiKey: machineAuthAPIKey,
	}
	return h.HandlePubKey
}

type pubKeyAuthHandler struct {
	ks     PubKeyStorage
	gw     *pool.Selector[gateway.GatewayAPIClient]
	apiKey string
}

func (h *pubKeyAuthHandler) HandlePubKey(ctx ssh.Context, key ssh.PublicKey) bool {
	userName := ctx.User()
	gw, err := h.gw.Next()
	if err != nil {
		return false
	}
	// Impersonate user to access his storage
	authRes, err := gw.Authenticate(context.Background(), &gateway.AuthenticateRequest{
		Type:         "machine",
		ClientId:     "username:" + userName,
		ClientSecret: h.apiKey,
	})

	if err != nil || authRes.Status.Code != rpc.Code_CODE_OK {
		return false
	}

	// Create authenticated context
	granteeCtx := ctxpkg.ContextSetUser(ctx, &userpb.User{Id: authRes.GetUser().GetId()})
	granteeCtx = metadata.AppendToOutgoingContext(granteeCtx, ctxpkg.TokenHeader, authRes.GetToken())
	granteeCtx = ctxpkg.ContextSetToken(granteeCtx, authRes.GetToken())

	availableKeys, err := h.ks.LoadKeys(granteeCtx, userName)
	if err != nil {
		return false
	}

	for _, storedKey := range availableKeys {
		if ssh.KeysEqual(storedKey, key) {
			ctx.SetValue("uid", authRes.GetUser().GetId())
			ctx.SetValue("token", authRes.GetToken())
			return true
		}
	}

	return false
}
