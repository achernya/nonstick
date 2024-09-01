package commands

import (
	"net/http"

	hydra "github.com/ory/hydra-client-go/v2"
)

type OryHydraFlow struct {
	client *hydra.APIClient
}

func NewOryHydraFlow() *OryHydraFlow {
	config := hydra.NewConfiguration()
	config.Servers[0].URL = "http://localhost:4445"
	return &OryHydraFlow{
		client: hydra.NewAPIClient(config),
	}
}

func (o *OryHydraFlow) acceptReq(username string) *hydra.AcceptOAuth2LoginRequest {
	req := hydra.NewAcceptOAuth2LoginRequest(username)
	req.SetRemember(true)
	req.SetRememberFor(30)
	return req
}

func (o *OryHydraFlow) PreLogin(r *http.Request) (string, error) {
	ctx := r.Context()

	// Ory Hydra should have included a `login_challenge` query
	// parameter if this is a legitimate login request.
	loginChallenge := r.URL.Query().Get("login_challenge")
	loginResp, _, err := o.client.OAuth2API.GetOAuth2LoginRequest(ctx).LoginChallenge(loginChallenge).Execute()
	if err != nil {
		return "", err
	}

	// We attemtped to get a new login request, but Hydra believes it's already authenticated.
	if loginResp.Skip {
		acceptResp, _, err := o.client.OAuth2API.AcceptOAuth2LoginRequest(ctx).
			LoginChallenge(loginChallenge).
			AcceptOAuth2LoginRequest(*o.acceptReq(loginResp.Subject)).
			Execute()
		if err != nil {
			return "", err
		}
		return acceptResp.RedirectTo, nil
	}

	// If we get here, authentication is required
	return "", nil
}

func (o *OryHydraFlow) Authenticated(r *http.Request, username string) (string, error) {
	ctx := r.Context()
	loginChallenge := r.URL.Query().Get("login_challenge")
	acceptResp, _, err := o.client.OAuth2API.AcceptOAuth2LoginRequest(ctx).
		LoginChallenge(loginChallenge).
		AcceptOAuth2LoginRequest(*o.acceptReq(username)).
		Execute()
	if err != nil {
		return "", err
	}
	return acceptResp.RedirectTo, nil
}
