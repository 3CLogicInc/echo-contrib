/* Package casbin provides middleware to enable ACL, RBAC, ABAC authorization support.

Simple example:

	package main

	import (
		"github.com/casbin/casbin/v2"
		"github.com/labstack/echo/v4"
		casbin_mw "github.com/labstack/echo-contrib/casbin"
	)

	func main() {
		e := echo.New()

		// Mediate the access for every request
		e.Use(casbin_mw.Middleware(casbin.NewEnforcer("auth_model.conf", "auth_policy.csv")))

		e.Logger.Fatal(e.Start(":1323"))
	}

Advanced example:

	package main

	import (
		"github.com/casbin/casbin/v2"
		"github.com/labstack/echo/v4"
		casbin_mw "github.com/labstack/echo-contrib/casbin"
	)

	func main() {
		ce, _ := casbin.NewEnforcer("auth_model.conf", "")
		ce.AddRoleForUser("alice", "admin")
		ce.AddPolicy(...)

		e := echo.New()

		e.Use(casbin_mw.Middleware(ce))

		e.Logger.Fatal(e.Start(":1323"))
	}
*/

package casbin

import (
	"encoding/base64"
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/casbin/casbin/v2"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

//AuthenticationType ...
type AuthenticationType int

const (
	//BasicAuth refers to basic authentication
	//default option
	BasicAuth AuthenticationType = iota
	//JwtAuth refers to JWT token auth in 'Authorization` header
	JwtAuth
)

type (
	// Config defines the config for CasbinAuth middleware.
	Config struct {
		// Skipper defines a function to skip middleware.
		Skipper middleware.Skipper

		// Enforcer CasbinAuth main rule.
		// Required.
		Enforcer *casbin.Enforcer

		//AuthType ...
		AuthType AuthenticationType
	}
)

var (
	// DefaultConfig is the default CasbinAuth middleware config.
	DefaultConfig = Config{
		Skipper: middleware.DefaultSkipper,
	}
)

//JWT registered claims as per
//https://tools.ietf.org/html/rfc7519#page-8
type jwtClaims struct {
	Iss string `json:"iss"`
	Sub string `json:"sub"`
	Aud string `json:"aud"`
	Exp string `json:"exp"`
	Nbf string `json:"nbf"`
	Iat string `json:"iat"`
	Jti string `json:"jti"`
}

// Middleware returns a CasbinAuth middleware.
//
// For valid credentials it calls the next handler.
// For missing or invalid credentials, it sends "401 - Unauthorized" response.
func Middleware(ce *casbin.Enforcer) echo.MiddlewareFunc {
	c := DefaultConfig
	c.Enforcer = ce
	return MiddlewareWithConfig(c)
}

// MiddlewareWithConfig returns a CasbinAuth middleware with config.
// See `Middleware()`.
func MiddlewareWithConfig(config Config) echo.MiddlewareFunc {
	// Defaults
	if config.Skipper == nil {
		config.Skipper = DefaultConfig.Skipper
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if config.Skipper(c) {
				return next(c)
			}

			if pass, err := config.CheckPermission(c); err == nil && pass {
				return next(c)
			} else if err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
			}

			return echo.ErrForbidden
		}
	}
}

// GetUserName gets the user name from the request.
// Currently, only HTTP basic authentication is supported
func (a *Config) GetUserName(c echo.Context) string {
	switch a.AuthType {
	case BasicAuth:
		return a.getUserNameBasicAuth(c)
	case JwtAuth:
		return a.getUserNameJwtAuth(c)
	}

	return ""
}

func (a *Config) getUserNameBasicAuth(c echo.Context) string {
	username, _, _ := c.Request().BasicAuth()
	return username
}

func (a *Config) getUserNameJwtAuth(c echo.Context) string {
	//parse 'sub' from the jwt token
	token := c.Request().Header.Get("Authorization")
	splitToken := strings.Split(token, ".")

	tokenBody := splitToken[1]
	body, err := base64.RawStdEncoding.DecodeString(tokenBody)
	if err != nil {
		log.Printf("token decode failed: %s", err)
		return ""
	}

	jsonBody := &jwtClaims{}
	json.Unmarshal(body, jsonBody)

	return jsonBody.Sub
}

// CheckPermission checks the user/method/path combination from the request.
// Returns true (permission granted) or false (permission forbidden)
func (a *Config) CheckPermission(c echo.Context) (bool, error) {
	user := a.GetUserName(c)
	method := c.Request().Method
	path := c.Request().URL.Path
	return a.Enforcer.Enforce(user, path, method)
}
