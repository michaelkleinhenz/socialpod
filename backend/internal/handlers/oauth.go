package handlers

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"socialmedia/internal/database"
	"socialmedia/internal/middleware"
	"socialmedia/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// OAuthHandler is the OAuth 2.1 / OpenID Connect authorization server that
// lets remote MCP clients — Claude.ai custom connectors (Pro, Max, Team and
// Enterprise plans), the Claude apps, Claude Code, ChatGPT connectors — sign a
// user in without a pasted API token. It implements what the MCP
// authorization spec asks for:
//
//   - protected resource metadata for /api/mcp (RFC 9728), advertised through
//     the WWW-Authenticate header of a 401 from the MCP endpoint
//   - authorization server metadata (RFC 8414) and OpenID discovery
//   - dynamic client registration (RFC 7591)
//   - the authorization code grant with mandatory PKCE S256, and refresh
//     tokens with rotation
//   - ID tokens (RS256, JWKS published) and a userinfo endpoint for clients
//     that ask for the openid scope
//
// The authorization endpoint is a page of the SPA (/oauth/authorize): the
// user signs in as usual and approves the client there, and the page calls
// AuthorizeDecision to obtain the redirect back to the client.
//
// Tokens issued here are scoped to the MCP endpoint, which exposes user
// features only; they are never accepted by the rest of the API, and they
// never carry admin rights, even for an admin.
type OAuthHandler struct {
	DB     *database.MongoDB
	Issuer string // public base URL (APP_URL), without trailing slash

	keyMu sync.Mutex
	key   *rsa.PrivateKey
	kid   string
}

const (
	oauthCodeTTL         = 10 * time.Minute
	oauthAccessTokenTTL  = time.Hour
	oauthRefreshTokenTTL = 90 * 24 * time.Hour
	oauthDefaultScope    = "mcp"
)

// oauthScopes are the scopes this server grants. "mcp" is the access to the
// MCP server itself; the OpenID scopes only shape the ID token and userinfo.
var oauthScopes = []string{"mcp", "openid", "profile", "email", "offline_access"}

func NewOAuthHandler(db *database.MongoDB, appURL string) *OAuthHandler {
	return &OAuthHandler{DB: db, Issuer: strings.TrimRight(appURL, "/")}
}

// ResourceURL is the canonical URL of the protected resource: the MCP server.
func (h *OAuthHandler) ResourceURL() string { return h.Issuer + "/api/mcp" }

// ResourceMetadataURL is where the MCP endpoint's 401 points clients to.
func (h *OAuthHandler) ResourceMetadataURL() string {
	return h.Issuer + "/.well-known/oauth-protected-resource/api/mcp"
}

// ── Discovery ────────────────────────────────────────────────────────────────

func (h *OAuthHandler) ProtectedResourceMetadata(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"resource":                 h.ResourceURL(),
		"authorization_servers":    []string{h.Issuer},
		"scopes_supported":         oauthScopes,
		"bearer_methods_supported": []string{"header"},
		"resource_name":            "SocialPod",
	})
}

// AuthorizationServerMetadata serves both the OAuth (RFC 8414) and the OpenID
// discovery document; the OpenID fields are a superset clients ignore if they
// don't need them.
func (h *OAuthHandler) AuthorizationServerMetadata(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"issuer":                                         h.Issuer,
		"authorization_endpoint":                         h.Issuer + "/oauth/authorize",
		"token_endpoint":                                 h.Issuer + "/api/oauth/token",
		"registration_endpoint":                          h.Issuer + "/api/oauth/register",
		"revocation_endpoint":                            h.Issuer + "/api/oauth/revoke",
		"userinfo_endpoint":                              h.Issuer + "/api/oauth/userinfo",
		"jwks_uri":                                       h.Issuer + "/api/oauth/jwks",
		"scopes_supported":                               oauthScopes,
		"response_types_supported":                       []string{"code"},
		"response_modes_supported":                       []string{"query"},
		"grant_types_supported":                          []string{"authorization_code", "refresh_token"},
		"code_challenge_methods_supported":               []string{"S256"},
		"token_endpoint_auth_methods_supported":          []string{"none", "client_secret_post", "client_secret_basic"},
		"revocation_endpoint_auth_methods_supported":     []string{"none", "client_secret_post", "client_secret_basic"},
		"subject_types_supported":                        []string{"public"},
		"id_token_signing_alg_values_supported":          []string{"RS256"},
		"claims_supported":                               []string{"sub", "iss", "aud", "exp", "iat", "auth_time", "nonce", "name", "email"},
		"authorization_response_iss_parameter_supported": true,
		"service_documentation":                          h.Issuer + "/profile",
	})
}

func (h *OAuthHandler) JWKS(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	key, kid, err := h.signingKey(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "signing key unavailable"})
		return
	}
	pub := key.PublicKey
	c.JSON(http.StatusOK, gin.H{"keys": []gin.H{{
		"kty": "RSA",
		"use": "sig",
		"alg": "RS256",
		"kid": kid,
		"n":   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
		"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
	}}})
}

// signingKey loads the ID token signing key, generating and storing one on
// first use. The oldest stored key wins, so instances racing to create one
// converge on the same key.
func (h *OAuthHandler) signingKey(ctx context.Context) (*rsa.PrivateKey, string, error) {
	h.keyMu.Lock()
	defer h.keyMu.Unlock()
	if h.key != nil {
		return h.key, h.kid, nil
	}

	oldest := options.FindOne().SetSort(bson.D{{Key: "createdAt", Value: 1}})
	var stored models.OAuthSigningKey
	err := h.DB.OAuthKeys().FindOne(ctx, bson.M{}, oldest).Decode(&stored)
	if errors.Is(err, mongo.ErrNoDocuments) {
		key, gerr := rsa.GenerateKey(rand.Reader, 2048)
		if gerr != nil {
			return nil, "", gerr
		}
		der := x509.MarshalPKCS1PrivateKey(key)
		stored = models.OAuthSigningKey{
			KID:           randomToken(12),
			PrivateKeyPEM: string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: der})),
			CreatedAt:     time.Now(),
		}
		if _, err = h.DB.OAuthKeys().InsertOne(ctx, stored); err != nil {
			return nil, "", err
		}
		err = h.DB.OAuthKeys().FindOne(ctx, bson.M{}, oldest).Decode(&stored)
	}
	if err != nil {
		return nil, "", err
	}

	block, _ := pem.Decode([]byte(stored.PrivateKeyPEM))
	if block == nil {
		return nil, "", errors.New("invalid signing key")
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, "", err
	}
	h.key, h.kid = key, stored.KID
	return h.key, h.kid, nil
}

// ── Dynamic client registration ─────────────────────────────────────────────

type clientRegistration struct {
	RedirectURIs            []string `json:"redirect_uris"`
	ClientName              string   `json:"client_name"`
	ClientURI               string   `json:"client_uri"`
	LogoURI                 string   `json:"logo_uri"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
}

func registrationError(c *gin.Context, code, desc string) {
	c.JSON(http.StatusBadRequest, gin.H{"error": code, "error_description": desc})
}

func (h *OAuthHandler) Register(c *gin.Context) {
	var req clientRegistration
	if err := c.ShouldBindJSON(&req); err != nil {
		registrationError(c, "invalid_client_metadata", "request body must be JSON client metadata")
		return
	}

	if len(req.RedirectURIs) == 0 || len(req.RedirectURIs) > 10 {
		registrationError(c, "invalid_redirect_uri", "between 1 and 10 redirect_uris are required")
		return
	}
	for _, u := range req.RedirectURIs {
		if len(u) > 2000 || !validRedirectURI(u) {
			registrationError(c, "invalid_redirect_uri", "redirect URI not allowed: "+u)
			return
		}
	}

	// RFC 7591 defaults to client_secret_basic when the method is omitted.
	method := req.TokenEndpointAuthMethod
	if method == "" {
		method = "client_secret_basic"
	}
	if method != "none" && method != "client_secret_post" && method != "client_secret_basic" {
		registrationError(c, "invalid_client_metadata", "unsupported token_endpoint_auth_method")
		return
	}

	grantTypes := []string{}
	for _, g := range req.GrantTypes {
		if g != "authorization_code" && g != "refresh_token" {
			registrationError(c, "invalid_client_metadata", "unsupported grant type: "+g)
			return
		}
		grantTypes = append(grantTypes, g)
	}
	if len(grantTypes) == 0 {
		grantTypes = []string{"authorization_code", "refresh_token"}
	}
	for _, rt := range req.ResponseTypes {
		if rt != "code" {
			registrationError(c, "invalid_client_metadata", "unsupported response type: "+rt)
			return
		}
	}

	client := models.OAuthClient{
		ClientID:                "spc_" + randomToken(24),
		ClientName:              truncate(strings.TrimSpace(req.ClientName), 200),
		ClientURI:               truncate(httpsOnly(req.ClientURI), 500),
		LogoURI:                 truncate(httpsOnly(req.LogoURI), 500),
		RedirectURIs:            req.RedirectURIs,
		TokenEndpointAuthMethod: method,
		GrantTypes:              grantTypes,
		CreatedAt:               time.Now(),
	}
	secret := ""
	if method != "none" {
		secret = "sps_" + randomToken(32)
		client.ClientSecretHash = middleware.HashOAuthToken(secret)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := h.DB.OAuthClients().InsertOne(ctx, client); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}

	resp := gin.H{
		"client_id":                  client.ClientID,
		"client_id_issued_at":        client.CreatedAt.Unix(),
		"client_name":                client.ClientName,
		"redirect_uris":              client.RedirectURIs,
		"token_endpoint_auth_method": client.TokenEndpointAuthMethod,
		"grant_types":                client.GrantTypes,
		"response_types":             []string{"code"},
	}
	if client.ClientURI != "" {
		resp["client_uri"] = client.ClientURI
	}
	if client.LogoURI != "" {
		resp["logo_uri"] = client.LogoURI
	}
	if secret != "" {
		resp["client_secret"] = secret
		resp["client_secret_expires_at"] = 0
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusCreated, resp)
}

// validRedirectURI accepts https URIs, http only on a loopback host (native
// apps and CLIs such as Claude Code, RFC 8252 §7.3), and private-use schemes
// of native apps (RFC 8252 §7.1). Fragments and script-capable schemes are
// refused.
func validRedirectURI(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Fragment != "" || u.Scheme == "" {
		return false
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
		return u.Host != ""
	case "http":
		return isLoopbackHost(u.Hostname())
	case "javascript", "data", "file", "vbscript", "about", "blob":
		return false
	default:
		return true
	}
}

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// redirectURIMatches compares a requested redirect URI with a registered one:
// exactly, except that a loopback http URI may use any port (RFC 8252 §7.3),
// since a native client picks a free port at runtime.
func redirectURIMatches(registered, requested string) bool {
	if registered == requested {
		return true
	}
	r, err1 := url.Parse(registered)
	q, err2 := url.Parse(requested)
	if err1 != nil || err2 != nil {
		return false
	}
	return r.Scheme == "http" && q.Scheme == "http" &&
		isLoopbackHost(r.Hostname()) && r.Hostname() == q.Hostname() &&
		r.Path == q.Path && r.RawQuery == q.RawQuery
}

// ── Authorization (consent) ─────────────────────────────────────────────────

type authorizeParams struct {
	ResponseType        string `json:"response_type" form:"response_type"`
	ClientID            string `json:"client_id" form:"client_id"`
	RedirectURI         string `json:"redirect_uri" form:"redirect_uri"`
	Scope               string `json:"scope" form:"scope"`
	State               string `json:"state" form:"state"`
	CodeChallenge       string `json:"code_challenge" form:"code_challenge"`
	CodeChallengeMethod string `json:"code_challenge_method" form:"code_challenge_method"`
	Resource            string `json:"resource" form:"resource"`
	Nonce               string `json:"nonce" form:"nonce"`
}

// authorizeError is a failed authorization request. A fatal one (unknown
// client, unregistered redirect URI) must never redirect, since the redirect
// target cannot be trusted; any other is reported to the client by redirect.
type authorizeError struct {
	fatal bool
	code  string
	desc  string
}

func (h *OAuthHandler) validateAuthorize(ctx context.Context, p *authorizeParams) (*models.OAuthClient, string, *authorizeError) {
	if p.ClientID == "" {
		return nil, "", &authorizeError{fatal: true, code: "invalid_request", desc: "client_id is required"}
	}
	var client models.OAuthClient
	if err := h.DB.OAuthClients().FindOne(ctx, bson.M{"clientId": p.ClientID}).Decode(&client); err != nil {
		return nil, "", &authorizeError{fatal: true, code: "invalid_client", desc: "Unknown client. Remove the connector and add it again."}
	}

	if p.RedirectURI == "" {
		if len(client.RedirectURIs) != 1 {
			return nil, "", &authorizeError{fatal: true, code: "invalid_request", desc: "redirect_uri is required"}
		}
		p.RedirectURI = client.RedirectURIs[0]
	}
	matched := false
	for _, r := range client.RedirectURIs {
		if redirectURIMatches(r, p.RedirectURI) {
			matched = true
			break
		}
	}
	if !matched {
		return nil, "", &authorizeError{fatal: true, code: "invalid_request", desc: "redirect_uri is not registered for this client"}
	}

	if p.ResponseType != "code" {
		return &client, "", &authorizeError{code: "unsupported_response_type", desc: "only response_type=code is supported"}
	}
	if p.CodeChallenge == "" {
		return &client, "", &authorizeError{code: "invalid_request", desc: "PKCE code_challenge is required"}
	}
	if p.CodeChallengeMethod != "S256" {
		return &client, "", &authorizeError{code: "invalid_request", desc: "code_challenge_method must be S256"}
	}

	return &client, normalizeScope(p.Scope), nil
}

// normalizeScope keeps the supported scopes of a request, in a stable order,
// and always includes the base "mcp" scope. Unknown scopes are dropped rather
// than failing the request, so clients that ask for extra scopes still work.
func normalizeScope(requested string) string {
	want := map[string]bool{oauthDefaultScope: true}
	for _, s := range strings.Fields(requested) {
		want[s] = true
	}
	out := []string{}
	for _, s := range oauthScopes {
		if want[s] {
			out = append(out, s)
		}
	}
	return strings.Join(out, " ")
}

func hasScope(scope, s string) bool {
	for _, f := range strings.Fields(scope) {
		if f == s {
			return true
		}
	}
	return false
}

// clientRedirect builds the redirect back to the client, keeping any query
// the registered URI already carries.
func (h *OAuthHandler) clientRedirect(redirectURI string, params map[string]string) string {
	u, err := url.Parse(redirectURI)
	if err != nil {
		return redirectURI
	}
	q := u.Query()
	for k, v := range params {
		if v != "" {
			q.Set(k, v)
		}
	}
	q.Set("iss", h.Issuer)
	u.RawQuery = q.Encode()
	return u.String()
}

// sessionUser returns the signed-in user approving an authorization request.
// Team tokens have no user behind them and cannot approve.
func (h *OAuthHandler) sessionUser(ctx context.Context, c *gin.Context) (*models.User, bool) {
	if isTeam, _ := c.Get("isTeamToken"); isTeam == true {
		return nil, false
	}
	userID, _ := c.Get("userId")
	uid, err := primitive.ObjectIDFromHex(userID.(string))
	if err != nil {
		return nil, false
	}
	var user models.User
	if err := h.DB.Users().FindOne(ctx, bson.M{"_id": uid}).Decode(&user); err != nil {
		return nil, false
	}
	return &user, true
}

// AuthorizeInfo validates an authorization request for the consent page and
// describes it. GET /api/oauth/authorize (signed-in user).
func (h *OAuthHandler) AuthorizeInfo(c *gin.Context) {
	var p authorizeParams
	_ = c.ShouldBindQuery(&p)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	user, ok := h.sessionUser(ctx, c)
	if !ok {
		c.JSON(http.StatusForbidden, gin.H{"error": "Sign in as a user to connect an app"})
		return
	}

	client, scope, aerr := h.validateAuthorize(ctx, &p)
	if aerr != nil {
		if aerr.fatal {
			c.JSON(http.StatusBadRequest, gin.H{"error": aerr.desc})
			return
		}
		c.JSON(http.StatusOK, gin.H{"redirectUrl": h.clientRedirect(p.RedirectURI, map[string]string{
			"error": aerr.code, "error_description": aerr.desc, "state": p.State,
		})})
		return
	}

	redirectHost := p.RedirectURI
	if u, err := url.Parse(p.RedirectURI); err == nil && u.Host != "" {
		redirectHost = u.Host
	}
	c.JSON(http.StatusOK, gin.H{
		"client": gin.H{
			"name":    client.ClientName,
			"uri":     client.ClientURI,
			"logoUri": client.LogoURI,
		},
		"redirectHost": redirectHost,
		"scopes":       strings.Fields(scope),
		"user":         gin.H{"name": user.Name, "email": user.Email},
	})
}

// AuthorizeDecision records the user's approval or denial and returns the
// redirect back to the client. POST /api/oauth/authorize (signed-in user).
func (h *OAuthHandler) AuthorizeDecision(c *gin.Context) {
	var body struct {
		authorizeParams
		Approve bool `json:"approve"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}
	p := body.authorizeParams

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	user, ok := h.sessionUser(ctx, c)
	if !ok {
		c.JSON(http.StatusForbidden, gin.H{"error": "Sign in as a user to connect an app"})
		return
	}

	_, scope, aerr := h.validateAuthorize(ctx, &p)
	if aerr != nil {
		if aerr.fatal {
			c.JSON(http.StatusBadRequest, gin.H{"error": aerr.desc})
			return
		}
		c.JSON(http.StatusOK, gin.H{"redirectUrl": h.clientRedirect(p.RedirectURI, map[string]string{
			"error": aerr.code, "error_description": aerr.desc, "state": p.State,
		})})
		return
	}

	if !body.Approve {
		c.JSON(http.StatusOK, gin.H{"redirectUrl": h.clientRedirect(p.RedirectURI, map[string]string{
			"error": "access_denied", "error_description": "The user denied access", "state": p.State,
		})})
		return
	}

	code := randomToken(32)
	now := time.Now()
	if _, err := h.DB.OAuthCodes().InsertOne(ctx, models.OAuthAuthCode{
		CodeHash:            middleware.HashOAuthToken(code),
		ClientID:            p.ClientID,
		UserID:              user.ID,
		RedirectURI:         p.RedirectURI,
		CodeChallenge:       p.CodeChallenge,
		CodeChallengeMethod: p.CodeChallengeMethod,
		Scope:               scope,
		Resource:            p.Resource,
		Nonce:               truncate(p.Nonce, 500),
		AuthTime:            now,
		ExpiresAt:           now.Add(oauthCodeTTL),
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create authorization"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"redirectUrl": h.clientRedirect(p.RedirectURI, map[string]string{
		"code": code, "state": p.State,
	})})
}

// ── Token endpoint ──────────────────────────────────────────────────────────

func tokenError(c *gin.Context, status int, code, desc string) {
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	c.JSON(status, gin.H{"error": code, "error_description": desc})
}

// authenticateClient identifies the client of a token or revocation request,
// from HTTP Basic credentials or form fields. A client registered with a
// secret must present it; a public client needs only its id.
func (h *OAuthHandler) authenticateClient(ctx context.Context, c *gin.Context) (*models.OAuthClient, bool) {
	clientID, secret, hasBasic := c.Request.BasicAuth()
	if hasBasic {
		// RFC 6749 §2.3.1: Basic credentials are form-urlencoded first.
		if v, err := url.QueryUnescape(clientID); err == nil {
			clientID = v
		}
		if v, err := url.QueryUnescape(secret); err == nil {
			secret = v
		}
	} else {
		clientID = c.PostForm("client_id")
		secret = c.PostForm("client_secret")
	}
	if clientID == "" {
		return nil, false
	}

	var client models.OAuthClient
	if err := h.DB.OAuthClients().FindOne(ctx, bson.M{"clientId": clientID}).Decode(&client); err != nil {
		return nil, false
	}
	if client.ClientSecretHash != "" {
		given := middleware.HashOAuthToken(secret)
		if secret == "" || subtle.ConstantTimeCompare([]byte(given), []byte(client.ClientSecretHash)) != 1 {
			return nil, false
		}
	}
	return &client, true
}

func (h *OAuthHandler) Token(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, ok := h.authenticateClient(ctx, c)
	if !ok {
		c.Header("WWW-Authenticate", `Basic realm="SocialPod"`)
		tokenError(c, http.StatusUnauthorized, "invalid_client", "client authentication failed")
		return
	}

	switch c.PostForm("grant_type") {
	case "authorization_code":
		h.exchangeCode(ctx, c, client)
	case "refresh_token":
		h.refresh(ctx, c, client)
	default:
		tokenError(c, http.StatusBadRequest, "unsupported_grant_type", "grant_type must be authorization_code or refresh_token")
	}
}

func (h *OAuthHandler) exchangeCode(ctx context.Context, c *gin.Context, client *models.OAuthClient) {
	code := c.PostForm("code")
	verifier := c.PostForm("code_verifier")
	if code == "" || verifier == "" {
		tokenError(c, http.StatusBadRequest, "invalid_request", "code and code_verifier are required")
		return
	}

	// Single use: the code is removed as it is redeemed.
	var ac models.OAuthAuthCode
	err := h.DB.OAuthCodes().FindOneAndDelete(ctx, bson.M{
		"codeHash":  middleware.HashOAuthToken(code),
		"expiresAt": bson.M{"$gt": time.Now()},
	}).Decode(&ac)
	if err != nil {
		tokenError(c, http.StatusBadRequest, "invalid_grant", "authorization code is invalid or expired")
		return
	}
	if ac.ClientID != client.ClientID {
		tokenError(c, http.StatusBadRequest, "invalid_grant", "authorization code was issued to another client")
		return
	}
	if ru := c.PostForm("redirect_uri"); ru != "" && ru != ac.RedirectURI {
		tokenError(c, http.StatusBadRequest, "invalid_grant", "redirect_uri does not match the authorization request")
		return
	}
	if !verifyPKCE(verifier, ac.CodeChallenge) {
		tokenError(c, http.StatusBadRequest, "invalid_grant", "PKCE verification failed")
		return
	}

	var user models.User
	if err := h.DB.Users().FindOne(ctx, bson.M{"_id": ac.UserID}).Decode(&user); err != nil {
		tokenError(c, http.StatusBadRequest, "invalid_grant", "user no longer exists")
		return
	}

	h.issueTokens(ctx, c, client, &user, ac.Scope, ac.Resource, ac.Nonce, ac.AuthTime)
}

func (h *OAuthHandler) refresh(ctx context.Context, c *gin.Context, client *models.OAuthClient) {
	rt := c.PostForm("refresh_token")
	if rt == "" {
		tokenError(c, http.StatusBadRequest, "invalid_request", "refresh_token is required")
		return
	}

	// Rotation: the presented refresh token is consumed and a new one issued.
	var old models.OAuthToken
	err := h.DB.OAuthTokens().FindOneAndDelete(ctx, bson.M{
		"tokenHash": middleware.HashOAuthToken(rt),
		"kind":      models.OAuthTokenRefresh,
		"clientId":  client.ClientID,
		"expiresAt": bson.M{"$gt": time.Now()},
	}).Decode(&old)
	if err != nil {
		tokenError(c, http.StatusBadRequest, "invalid_grant", "refresh token is invalid or expired")
		return
	}

	var user models.User
	if err := h.DB.Users().FindOne(ctx, bson.M{"_id": old.UserID}).Decode(&user); err != nil {
		tokenError(c, http.StatusBadRequest, "invalid_grant", "user no longer exists")
		return
	}

	// A refresh may narrow the scope, never widen it.
	scope := old.Scope
	if req := c.PostForm("scope"); req != "" {
		narrowed := []string{}
		for _, s := range strings.Fields(req) {
			if hasScope(old.Scope, s) {
				narrowed = append(narrowed, s)
			}
		}
		if len(narrowed) == 0 {
			tokenError(c, http.StatusBadRequest, "invalid_scope", "requested scope exceeds the original grant")
			return
		}
		scope = strings.Join(narrowed, " ")
	}

	h.issueTokens(ctx, c, client, &user, scope, old.Resource, "", time.Time{})
}

func (h *OAuthHandler) issueTokens(ctx context.Context, c *gin.Context, client *models.OAuthClient, user *models.User, scope, resource, nonce string, authTime time.Time) {
	now := time.Now()
	access := middleware.OAuthAccessTokenPrefix + randomToken(32)
	refresh := middleware.OAuthRefreshTokenPrefix + randomToken(32)

	_, err := h.DB.OAuthTokens().InsertMany(ctx, []any{
		models.OAuthToken{
			TokenHash: middleware.HashOAuthToken(access), Kind: models.OAuthTokenAccess,
			ClientID: client.ClientID, UserID: user.ID, Scope: scope, Resource: resource,
			CreatedAt: now, ExpiresAt: now.Add(oauthAccessTokenTTL),
		},
		models.OAuthToken{
			TokenHash: middleware.HashOAuthToken(refresh), Kind: models.OAuthTokenRefresh,
			ClientID: client.ClientID, UserID: user.ID, Scope: scope, Resource: resource,
			CreatedAt: now, ExpiresAt: now.Add(oauthRefreshTokenTTL),
		},
	})
	if err != nil {
		tokenError(c, http.StatusInternalServerError, "server_error", "failed to issue tokens")
		return
	}

	resp := gin.H{
		"access_token":  access,
		"token_type":    "Bearer",
		"expires_in":    int(oauthAccessTokenTTL.Seconds()),
		"refresh_token": refresh,
		"scope":         scope,
	}

	if hasScope(scope, "openid") {
		idToken, err := h.idToken(ctx, client.ClientID, user, scope, nonce, authTime)
		if err != nil {
			tokenError(c, http.StatusInternalServerError, "server_error", "failed to sign ID token")
			return
		}
		resp["id_token"] = idToken
	}

	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	c.JSON(http.StatusOK, resp)
}

func (h *OAuthHandler) idToken(ctx context.Context, clientID string, user *models.User, scope, nonce string, authTime time.Time) (string, error) {
	key, kid, err := h.signingKey(ctx)
	if err != nil {
		return "", err
	}
	now := time.Now()
	claims := jwt.MapClaims{
		"iss": h.Issuer,
		"sub": user.ID.Hex(),
		"aud": clientID,
		"iat": now.Unix(),
		"exp": now.Add(oauthAccessTokenTTL).Unix(),
	}
	if !authTime.IsZero() {
		claims["auth_time"] = authTime.Unix()
	}
	if nonce != "" {
		claims["nonce"] = nonce
	}
	for k, v := range userClaims(user, scope) {
		claims[k] = v
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = kid
	return tok.SignedString(key)
}

func userClaims(user *models.User, scope string) map[string]any {
	out := map[string]any{}
	if hasScope(scope, "profile") {
		out["name"] = user.Name
	}
	if hasScope(scope, "email") {
		out["email"] = user.Email
	}
	return out
}

func verifyPKCE(verifier, challenge string) bool {
	if len(verifier) < 43 || len(verifier) > 128 {
		return false
	}
	sum := sha256.Sum256([]byte(verifier))
	computed := base64.RawURLEncoding.EncodeToString(sum[:])
	return subtle.ConstantTimeCompare([]byte(computed), []byte(challenge)) == 1
}

// Revoke implements RFC 7009. Revoking a refresh token ends the whole grant
// for that client, revoking an access token only that token. The response is
// 200 whether or not the token existed.
func (h *OAuthHandler) Revoke(c *gin.Context) {
	token := c.PostForm("token")
	if token == "" {
		tokenError(c, http.StatusBadRequest, "invalid_request", "token is required")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var tok models.OAuthToken
	if err := h.DB.OAuthTokens().FindOneAndDelete(ctx, bson.M{"tokenHash": middleware.HashOAuthToken(token)}).Decode(&tok); err == nil {
		if tok.Kind == models.OAuthTokenRefresh {
			h.DB.OAuthTokens().DeleteMany(ctx, bson.M{"userId": tok.UserID, "clientId": tok.ClientID})
		}
	}
	c.Status(http.StatusOK)
}

// UserInfo is the OpenID Connect userinfo endpoint, authenticated with an
// OAuth access token.
func (h *OAuthHandler) UserInfo(c *gin.Context) {
	tokenStr := strings.TrimSpace(strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer "))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	user, tok, err := middleware.LookupOAuthAccessToken(ctx, h.DB, tokenStr)
	if err != nil {
		c.Header("WWW-Authenticate", `Bearer error="invalid_token"`)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_token"})
		return
	}
	resp := gin.H{"sub": user.ID.Hex()}
	for k, v := range userClaims(user, tok.Scope) {
		resp[k] = v
	}
	c.JSON(http.StatusOK, resp)
}

// ── Connected apps (Profile page) ───────────────────────────────────────────

// ListGrants lists the apps the signed-in user has connected through OAuth.
// GET /api/oauth/grants
func (h *OAuthHandler) ListGrants(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	user, ok := h.sessionUser(ctx, c)
	if !ok {
		c.JSON(http.StatusOK, []gin.H{})
		return
	}

	cur, err := h.DB.OAuthTokens().Find(ctx, bson.M{
		"userId":    user.ID,
		"kind":      models.OAuthTokenRefresh,
		"expiresAt": bson.M{"$gt": time.Now()},
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list connected apps"})
		return
	}
	var tokens []models.OAuthToken
	if err := cur.All(ctx, &tokens); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list connected apps"})
		return
	}

	lastUsed := map[string]time.Time{}
	for _, t := range tokens {
		if t.CreatedAt.After(lastUsed[t.ClientID]) {
			lastUsed[t.ClientID] = t.CreatedAt
		}
	}

	grants := []gin.H{}
	for clientID, at := range lastUsed {
		name := ""
		var client models.OAuthClient
		if err := h.DB.OAuthClients().FindOne(ctx, bson.M{"clientId": clientID}).Decode(&client); err == nil {
			name = client.ClientName
		}
		grants = append(grants, gin.H{"clientId": clientID, "clientName": name, "lastUsedAt": at})
	}
	sort.Slice(grants, func(i, j int) bool {
		return grants[i]["lastUsedAt"].(time.Time).After(grants[j]["lastUsedAt"].(time.Time))
	})
	c.JSON(http.StatusOK, grants)
}

// RevokeGrant disconnects an app: every token it holds for the user is
// deleted. DELETE /api/oauth/grants/:clientId
func (h *OAuthHandler) RevokeGrant(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	user, ok := h.sessionUser(ctx, c)
	if !ok {
		c.JSON(http.StatusForbidden, gin.H{"error": "Sign in as a user"})
		return
	}
	if _, err := h.DB.OAuthTokens().DeleteMany(ctx, bson.M{"userId": user.ID, "clientId": c.Param("clientId")}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to disconnect app"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Disconnected"})
}

// ── helpers ─────────────────────────────────────────────────────────────────

func randomToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func truncate(s string, max int) string {
	if len(s) > max {
		return s[:max]
	}
	return s
}

func httpsOnly(raw string) string {
	if u, err := url.Parse(raw); err == nil && u.Scheme == "https" && u.Host != "" {
		return raw
	}
	return ""
}
