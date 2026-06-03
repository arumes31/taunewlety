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
					session.Set("user", "admin")
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
