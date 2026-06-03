package http

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSetupRouter(t *testing.T) {
	tests := []struct {
		name          string
		sessionSecret string
		env           string
		cookieSecure  string
		expectNil     bool
		expectFatal   bool
	}{
		{
			name:          "Success with normal env",
			sessionSecret: "test-secret",
			expectNil:     false,
		},
		{
			name:          "Success with production env",
			sessionSecret: "test-secret",
			env:           "production",
			expectNil:     false,
		},
		{
			name:          "Success with COOKIE_SECURE=true",
			sessionSecret: "test-secret",
			cookieSecure:  "true",
			expectNil:     false,
		},
		{
			name:          "Missing session secret",
			sessionSecret: "",
			expectNil:     true,
			expectFatal:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock logFatal
			oldFatal := logFatal
			var fatalCalled bool
			logFatal = func(v ...interface{}) {
				fatalCalled = true
			}
			defer func() { logFatal = oldFatal }()

			// Set env vars
			if tt.sessionSecret != "" {
				os.Setenv("SESSION_SECRET", tt.sessionSecret)
			} else {
				os.Unsetenv("SESSION_SECRET")
			}
			os.Setenv("ENV", tt.env)
			os.Setenv("COOKIE_SECURE", tt.cookieSecure)

			defer func() {
				os.Unsetenv("SESSION_SECRET")
				os.Unsetenv("ENV")
				os.Unsetenv("COOKIE_SECURE")
			}()

			router := SetupRouter()

			if tt.expectNil && router != nil {
				t.Error("expected nil router")
			}
			if !tt.expectNil && router == nil {
				t.Error("expected non-nil router")
			}
			if tt.expectFatal && !fatalCalled {
				t.Error("expected log.Fatal to be called")
			}
		})
	}
}

func TestResolveWebDir(t *testing.T) {
	tests := []struct {
		name     string
		getwdErr error
		mockWd   string
		setupFs  func(string) string // returns temporary base dir
		expected string
	}{
		{
			name:     "getwd error returns web",
			getwdErr: errors.New("error"),
			expected: "web",
		},
		{
			name:   "found in current dir",
			mockWd: t.TempDir(),
			setupFs: func(base string) string {
				path := filepath.Join(base, "web", "template")
				_ = os.MkdirAll(path, 0755)
				return base
			},
			expected: "found_in_setup", // will be replaced in test loop
		},
		{
			name:   "found in parent dir",
			mockWd: "", // will be set in setupFs
			setupFs: func(base string) string {
				// base/web/template
				// base/subdir/wd
				parent := t.TempDir()
				_ = os.MkdirAll(filepath.Join(parent, "web", "template"), 0755)
				wd := filepath.Join(parent, "subdir")
				_ = os.MkdirAll(wd, 0755)
				return wd
			},
			expected: "parent", // will be replaced
		},
		{
			name:   "reached root without finding",
			mockWd: "",
			setupFs: func(base string) string {
				// Use a path that definitely won't have web/template up to root
				// On Windows this might be tricky, but we can mock it
				return base
			},
			expected: "web",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldGetwd := getwd
			defer func() { getwd = oldGetwd }()

			wd := tt.mockWd
			if tt.setupFs != nil {
				res := tt.setupFs(t.TempDir())
				if tt.name == "found in current dir" {
					tt.expected = filepath.Join(res, "web")
				} else if tt.name == "found in parent dir" {
					tt.expected = filepath.Join(filepath.Dir(res), "web")
				}
				wd = res
			}

			getwd = func() (string, error) {
				return wd, tt.getwdErr
			}

			result := resolveWebDir()
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestResolveWebDir_RootPath_Mock(t *testing.T) {
	oldGetwd := getwd
	defer func() { getwd = oldGetwd }()

	getwd = func() (string, error) {
		if runtime.GOOS == "windows" {
			return `C:\`, nil
		}
		return "/", nil
	}

	dir := resolveWebDir()
	if dir != "web" {
		t.Errorf("expected 'web' when starting at root, got: %s", dir)
	}
}
