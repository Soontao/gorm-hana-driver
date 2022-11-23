package driver

import (
	"strings"
	"sync"

	p "github.com/SAP/go-hdb/driver/internal/protocol"
	"github.com/SAP/go-hdb/driver/internal/protocol/x509"
)

// authAttrs is holding authentication relevant attributes.
type authAttrs struct {
	hasCookie            atomicBool
	mu                   sync.RWMutex
	_username, _password string        // basic authentication
	_certKey             *x509.CertKey // X509
	_token               string        // JWT
	_logonname           string        // session cookie login does need logon name provided by JWT authentication.
	_sessionCookie       []byte        // authentication via session cookie (HDB currently does support only SAML and JWT - go-hdb JWT)
	_refreshPassword     func() (password string, ok bool)
	_refreshClientCert   func() (clientCert, clientKey []byte, ok bool)
	_refreshToken        func() (token string, ok bool)
}

/*
	keep c as the instance name, so that the generated help does have
	the same instance variable name when included in connector
*/

func isJWTToken(token string) bool { return strings.HasPrefix(token, "ey") }

func (c *authAttrs) cookieAuth() *p.Auth {
	if !c.hasCookie.Load() { // fastpath without lock
		return nil
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	auth := p.NewAuth(c._logonname)                                 // important: for session cookie auth we do need the logonname from JWT auth,
	auth.AddSessionCookie(c._sessionCookie, c._logonname, clientID) // and for HANA onPrem the final session cookie req needs the logonname as well.
	return auth
}

func (c *authAttrs) auth() *p.Auth {
	c.mu.RLock()
	defer c.mu.RUnlock()

	auth := p.NewAuth(c._username) // use username as logonname
	if c._certKey != nil {
		auth.AddX509(c._certKey)
	}
	if c._token != "" {
		auth.AddJWT(c._token)
	}
	// mimic standard drivers and use password as token if user is empty
	if c._token == "" && c._username == "" && isJWTToken(c._password) {
		auth.AddJWT(c._password)
	}
	if c._password != "" {
		auth.AddBasic(c._username, c._password)
	}
	return auth
}

func (c *authAttrs) refreshPassword(passwordSetter p.AuthPasswordSetter) (bool, error) {
	password := c.Password()     // save password
	c.mu.RLock()                 // read lock as callback function might call connector read functions
	if password != c._password { // if password has changed it got refreshed in the meanwhile by another connection
		c.mu.RUnlock()
		return true, nil
	}
	if c._refreshPassword != nil {
		if password, ok := c._refreshPassword(); ok && c._password != password {
			c.mu.RUnlock()
			c.mu.Lock()
			c._password = password
			passwordSetter.SetPassword(password)
			c.mu.Unlock()
			return true, nil
		}
	}
	c.mu.RUnlock()
	return false, nil
}

func (c *authAttrs) refreshToken(tokenSetter p.AuthTokenSetter) (bool, error) {
	token := c.Token()     // save token
	c.mu.RLock()           // read lock as callback function might call connector read functions
	if token != c._token { // if token has changed it got refreshed in the meanwhile by another connection
		c.mu.RUnlock()
		return true, nil
	}
	if c._refreshToken != nil {
		if token, ok := c._refreshToken(); ok && c._token != token {
			c.mu.RUnlock()
			c.mu.Lock()
			c._token = token
			tokenSetter.SetToken(token)
			c.mu.Unlock()
			return true, nil
		}
	}
	c.mu.RUnlock()
	return false, nil
}

func (c *authAttrs) refreshCertKey(certKeySetter p.AuthCertKeySetter) (bool, error) {
	certKey := c._certKey      // save certKey
	c.mu.RLock()               // read lock as callback function might call connector read functions
	if certKey != c._certKey { // if certKey has changed it got refreshed in the meanwhile by another connection
		c.mu.RUnlock()
		return true, nil
	}
	if c._refreshClientCert != nil {
		if clientCert, clientKey, ok := c._refreshClientCert(); ok && !c._certKey.Equal(clientCert, clientKey) {
			c.mu.RUnlock()
			c.mu.Lock()
			certKey, err := x509.NewCertKey(clientCert, clientKey)
			if err != nil {
				c.mu.Unlock()
				return false, err
			}
			c._certKey = certKey
			certKeySetter.SetCertKey(certKey)
			c.mu.Unlock()
			return true, nil
		}
	}
	c.mu.RUnlock()
	return false, nil
}

func (c *authAttrs) refresh(auth *p.Auth) (bool, error) {
	switch method := auth.Method().(type) {

	case p.AuthPasswordSetter:
		return c.refreshPassword(method)
	case p.AuthTokenSetter:
		return c.refreshToken(method)
	case p.AuthCertKeySetter:
		return c.refreshCertKey(method)
	default:
		return false, nil
	}
}

func (c *authAttrs) invalidateCookie() { c.hasCookie.Store(false) }

func (c *authAttrs) setCookie(logonname string, sessionCookie []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.hasCookie.Store(true)
	c._logonname = logonname
	c._sessionCookie = sessionCookie
}

// Username returns the username of the connector.
func (c *authAttrs) Username() string { c.mu.RLock(); defer c.mu.RUnlock(); return c._username }

// Password returns the basic authentication password of the connector.
func (c *authAttrs) Password() string { c.mu.RLock(); defer c.mu.RUnlock(); return c._password }

// SetPassword sets the basic authentication password of the connector.
func (c *authAttrs) SetPassword(password string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c._password = password
}

// RefreshPassword returns the callback function for basic authentication password refresh.
func (c *authAttrs) RefreshPassword() func() (password string, ok bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c._refreshPassword
}

// SetRefreshPassword sets the callback function for basic authentication password refresh.
// The callback function might be called simultaneously from multiple goroutines if registered
// for more than one Connector.
func (c *authAttrs) SetRefreshPassword(refreshPassword func() (password string, ok bool)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c._refreshPassword = refreshPassword
}

// ClientCert returns the X509 authentication client certificate and key of the connector.
func (c *authAttrs) ClientCert() (clientCert, clientKey []byte) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c._certKey.Cert(), c._certKey.Key()
}

// RefreshClientCert returns the callback function for X509 authentication client certificate and key refresh.
func (c *authAttrs) RefreshClientCert() func() (clientCert, clientKey []byte, ok bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c._refreshClientCert
}

// SetRefreshClientCert sets the callback function for X509 authentication client certificate and key refresh.
// The callback function might be called simultaneously from multiple goroutines if registered
// for more than one Connector.
func (c *authAttrs) SetRefreshClientCert(refreshClientCert func() (clientCert, clientKey []byte, ok bool)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c._refreshClientCert = refreshClientCert
}

// Token returns the JWT authentication token of the connector.
func (c *authAttrs) Token() string { c.mu.RLock(); defer c.mu.RUnlock(); return c._token }

// RefreshToken returns the callback function for JWT authentication token refresh.
func (c *authAttrs) RefreshToken() func() (token string, ok bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c._refreshToken
}

// SetRefreshToken sets the callback function for JWT authentication token refresh.
// The callback function might be called simultaneously from multiple goroutines if registered
// for more than one Connector.
func (c *authAttrs) SetRefreshToken(refreshToken func() (token string, ok bool)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c._refreshToken = refreshToken
}