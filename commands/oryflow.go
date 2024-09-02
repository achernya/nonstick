package commands

import (
	"encoding/json"
	"net/http"

	"github.com/rs/zerolog/log"

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

func (o *OryHydraFlow) RequestConsent(r *http.Request) error {
	ctx := r.Context()
	consentChallenge := r.URL.Query().Get("consent_challenge")
	consentResp, _, err := o.client.OAuth2API.GetOAuth2ConsentRequest(ctx).
		ConsentChallenge(consentChallenge).
		Execute()
	if err != nil {
		return err
	}
	if s, err := json.MarshalIndent(consentResp, "", "  "); err == nil {
		log.Info().Msg(string(s))
	}

	// TODO(achernya): Return a structure with the requested scopes
	return nil
}

func (o *OryHydraFlow) AcceptConsent(r *http.Request) (string, error) {
	ctx := r.Context()
	consentChallenge := r.URL.Query().Get("consent_challenge")

	// TODO(achernya): Read this from the user's submission instead
	consentResp, _, err := o.client.OAuth2API.GetOAuth2ConsentRequest(ctx).
		ConsentChallenge(consentChallenge).
		Execute()
	if err != nil {
		return "", err
	}

	req := hydra.NewAcceptOAuth2ConsentRequest()
	req.SetGrantScope(consentResp.RequestedScope)
	req.SetGrantAccessTokenAudience(consentResp.RequestedAccessTokenAudience)
	req.SetRemember(true)
	req.SetRememberFor(3600)
	acceptResp, _, err := o.client.OAuth2API.AcceptOAuth2ConsentRequest(ctx).
		ConsentChallenge(consentChallenge).
		AcceptOAuth2ConsentRequest(*req).
		Execute()
	if err != nil {
		return "", err
	}

	return acceptResp.RedirectTo, nil
}
