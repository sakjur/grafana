package middleware

import (
	"bytes"
	"fmt"
	"net/http"
	"text/template"
	"time"

	"github.com/grafana/grafana/pkg/util"

	macaron "gopkg.in/macaron.v1"

	"github.com/grafana/grafana/pkg/models"
	"github.com/grafana/grafana/pkg/services/accesscontrol"
)

func Middleware(ac accesscontrol.AccessControl) func(macaron.Handler, string, ...string) macaron.Handler {
	return func(fallback macaron.Handler, permission string, scopes ...string) macaron.Handler {
		if ac.IsDisabled() {
			return fallback
		}

		return func(c *models.ReqContext) {
			// We need this otherwise templated scopes get initialized only once, during the first call
			runtimeScope := make([]string, len(scopes))
			for i, scope := range scopes {
				var buf bytes.Buffer

				tmpl, err := template.New("scope").Parse(scope)
				if err != nil {
					c.JsonApiErr(http.StatusInternalServerError, "Internal server error", err)
					return
				}
				err = tmpl.Execute(&buf, c.AllParams())
				if err != nil {
					c.JsonApiErr(http.StatusInternalServerError, "Internal server error", err)
					return
				}
				runtimeScope[i] = buf.String()
			}

			hasAccess, err := ac.Evaluate(c.Req.Context(), c.SignedInUser, permission, runtimeScope...)
			if err != nil {
				deny(c, permission, runtimeScope, err)
				return
			}
			if !hasAccess {
				deny(c, permission, runtimeScope, nil)
				return
			}
		}
	}
}

func deny(c *models.ReqContext, permission string, scopes []string, err error) {
	id := newID()
	if err != nil {
		c.Logger.Error("Error from access control system", "error", err, "accessErrorID", id)
	} else {
		c.Logger.Info("Access denied",
			"userID", c.UserId,
			"permission", permission,
			"scopes", scopes,
			"accessErrorID", id)
	}

	// If the user triggers an error in the access control system, we
	// don't want the user to be aware of that, so the user gets the
	// same information from the system regardless of if it's an
	// internal server error or access denied.
	c.JSON(http.StatusForbidden, map[string]string{
		"message":       fmt.Sprintf("Access denied. [Access error ID: %s]", id),
		"accessErrorId": id,
	})
}

func newID() string {
	// Less ambiguity than alphanumerical.
	numerical := []byte("0123456789")
	id, err := util.GetRandomString(10, numerical...)
	if err != nil {
		// this should not happen, but if it does, a timestamp is as
		// useful as anything.
		id = fmt.Sprintf("%d", time.Now().UnixNano())
	}
	id = "ACE" + id
}
