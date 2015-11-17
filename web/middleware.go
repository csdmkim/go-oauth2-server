package web

import (
	"net/http"

	"github.com/RichardKnop/go-oauth2-server/session"
	"github.com/gorilla/context"
)

// parseFormMiddleware parses the form so r.Form becomes available
type parseFormMiddleware struct{}

// ServeHTTP as per the negroni.Handler interface
func (m *parseFormMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	next(w, r)
}

// guestMiddleware just initialises session
type guestMiddleware struct{}

// ServeHTTP as per the negroni.Handler interface
func (m *guestMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	// Initialise the session service
	sessionService := session.NewService(theService.cnf, r, w)

	// Attempt to start the session
	if err := sessionService.StartUserSession(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	context.Set(r, sessionServiceKey, sessionService)

	next(w, r)
}

// loggedInMiddleware initialises session and makes sure the user is logged in
type loggedInMiddleware struct{}

// ServeHTTP as per the negroni.Handler interface
func (m *loggedInMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	// Initialise the session service
	sessionService := session.NewService(theService.cnf, r, w)

	// Attempt to start the session
	if err := sessionService.StartUserSession(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	context.Set(r, sessionServiceKey, sessionService)

	// Try to get a user session
	userSession, err := sessionService.GetUserSession()
	if err != nil {
		redirectWithQueryString("/web/login", r.URL.Query(), w, r)
		return
	}

	// Authenticate
	if err := authenticate(userSession); err != nil {
		redirectWithQueryString("/web/login", r.URL.Query(), w, r)
		return
	}

	// Update the user session
	sessionService.SetUserSession(userSession)

	next(w, r)
}

// clientMiddleware takes client_id param from the query string and
// makes a database lookup for a client with the same client ID
type clientMiddleware struct{}

// ServeHTTP as per the negroni.Handler interface
func (m *clientMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	// Fetch the client
	client, err := theService.oauthService.FindClientByClientID(
		r.Form.Get("client_id"), // client ID
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	context.Set(r, clientKey, client)

	next(w, r)
}

func authenticate(userSession *session.UserSession) error {
	// Try to authenticate with the stored access token
	err := theService.oauthService.Authenticate(userSession.AccessToken)
	if err == nil {
		// Access token valid, return
		return nil
	}
	// Access token might be expired, let's try refreshing...

	// Fetch the client
	client, err := theService.oauthService.FindClientByClientID(
		userSession.ClientID, // client ID
	)
	if err != nil {
		return err
	}

	// Validate the refresh token
	theRefreshToken, err := theService.oauthService.ValidateRefreshToken(
		userSession.RefreshToken, // refresh token
		client, // client
	)
	if err != nil {
		return err
	}

	// Create a new access token
	accessToken, err := theService.oauthService.GrantAccessToken(
		theRefreshToken.Client, // client
		theRefreshToken.User,   // user
		theRefreshToken.Scope,  // scope
	)
	if err != nil {
		return err
	}

	// Create or retrieve a refresh token
	refreshToken, err := theService.oauthService.GetOrCreateRefreshToken(
		theRefreshToken.Client, // client
		theRefreshToken.User,   // user
		theRefreshToken.Scope,  // scope
	)
	if err != nil {
		return err
	}

	userSession.AccessToken = accessToken.Token
	userSession.RefreshToken = refreshToken.Token

	return nil
}
