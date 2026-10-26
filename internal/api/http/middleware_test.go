package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
)

func TestAuthRequired_Middleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		setUser        bool
		revoke         bool
		userOnly       bool // pre-registry cookie: "user" set, no session ID
		expectedStatus int
		expectedBody   string
		expectedLoc    string
	}{
		{
			name:           "Unauthorized redirects to /login",
			setUser:        false,
			expectedStatus: http.StatusFound,
			expectedLoc:    "/login",
		},
		{
			name:           "Authorized allows access",
			setUser:        true,
			expectedStatus: http.StatusOK,
			expectedBody:   "welcome",
		},
		{
			name:           "Revoked session redirects to /login",
			setUser:        true,
			revoke:         true,
			expectedStatus: http.StatusFound,
			expectedLoc:    "/login",
		},
		{
			name:           "User without registered session ID redirects",
			setUser:        true,
			userOnly:       true,
			expectedStatus: http.StatusFound,
			expectedLoc:    "/login",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			store := cookie.NewStore([]byte("secret"))
			r.Use(sessions.Sessions("mysession", store))

			r.GET("/protected", AuthRequired(), func(c *gin.Context) {
				c.String(http.StatusOK, "welcome")
			})

			r.GET("/login", func(c *gin.Context) {
				c.String(http.StatusOK, "login page")
			})

			var cookieVal string
			if tt.setUser {
				r.GET("/set-session", func(c *gin.Context) {
					session := sessions.Default(c)
					if tt.userOnly {
						session.Set("user", "admin")
					} else if err := startAdminSession(session, "admin"); err != nil {
						t.Fatalf("startAdminSession: %v", err)
					}
					if tt.revoke {
						endAdminSession(session)
					}
					_ = session.Save()
					c.String(http.StatusOK, "session set")
				})

				wSet := httptest.NewRecorder()
				reqSet, _ := http.NewRequest(http.MethodGet, "/set-session", nil)
				r.ServeHTTP(wSet, reqSet)
				cookieVal = wSet.Header().Get("Set-Cookie")
			}

			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodGet, "/protected", nil)
			if cookieVal != "" {
				req.Header.Set("Cookie", cookieVal)
			}
			r.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got: %d", tt.expectedStatus, w.Code)
			}

			if tt.expectedLoc != "" {
				loc := w.Header().Get("Location")
				if loc != tt.expectedLoc {
					t.Errorf("expected redirect location %s, got: %s", tt.expectedLoc, loc)
				}
			}

			if tt.expectedBody != "" {
				if w.Body.String() != tt.expectedBody {
					t.Errorf("expected body %s, got: %s", tt.expectedBody, w.Body.String())
				}
			}
		})
	}
}
