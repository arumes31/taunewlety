package pkg

import (
	"crypto/rand"
	"errors"
	"fmt"
	"testing"
)

// mockReader allows mocking the global rand.Reader
type mockReader struct {
	readFunc func(p []byte) (n int, err error)
}

func (m mockReader) Read(p []byte) (n int, err error) {
	return m.readFunc(p)
}

func TestGenerateCaptcha(t *testing.T) {
	// Save the original reader and restore it after tests
	oldReader := rand.Reader
	defer func() { rand.Reader = oldReader }()

	tests := []struct {
		name    string
		setup   func()
		wantErr bool
	}{
		{
			name: "Success",
			setup: func() {
				rand.Reader = oldReader
			},
			wantErr: false,
		},
		{
			name: "Error on first random number generation",
			setup: func() {
				rand.Reader = mockReader{
					readFunc: func(p []byte) (int, error) {
						return 0, errors.New("forced error 1")
					},
				}
			},
			wantErr: true,
		},
		{
			name: "Error on second random number generation",
			setup: func() {
				firstCall := true
				rand.Reader = mockReader{
					readFunc: func(p []byte) (int, error) {
						if firstCall {
							if len(p) > 0 {
								p[0] = 5 // Ensure first rand.Int succeeds immediately
								firstCall = false
								return 1, nil
							}
						}
						return 0, errors.New("forced error 2")
					},
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			got, err := GenerateCaptcha()
			if (err != nil) != tt.wantErr {
				t.Errorf("GenerateCaptcha() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				// Basic sanity checks
				if got.Question == "" {
					t.Error("GenerateCaptcha() Question should not be empty")
				}

				// Verify the format and math
				var a, b int
				n, err := fmt.Sscanf(got.Question, "%d + %d", &a, &b)
				if err != nil || n != 2 {
					t.Fatalf("GenerateCaptcha() Question %q format is invalid: %v", got.Question, err)
				}

				if got.Answer != a+b {
					t.Errorf("GenerateCaptcha() Answer = %d, want %d", got.Answer, a+b)
				}

				// Verify ranges (1 to 10 inclusive)
				if a < 1 || a > 10 {
					t.Errorf("First number %d out of range [1, 10]", a)
				}
				if b < 1 || b > 10 {
					t.Errorf("Second number %d out of range [1, 10]", b)
				}
			}
		})
	}
}

func TestGenerateCaptcha_MultipleRuns(t *testing.T) {
	// Run multiple times to ensure stability and variety (though random, it's good to check it doesn't crash)
	for i := 0; i < 100; i++ {
		t.Run(fmt.Sprintf("Run %d", i), func(t *testing.T) {
			got, err := GenerateCaptcha()
			if err != nil {
				t.Fatalf("GenerateCaptcha() failed: %v", err)
			}
			var a, b int
			if _, err := fmt.Sscanf(got.Question, "%d + %d", &a, &b); err != nil {
				t.Errorf("failed to parse question %q: %v", got.Question, err)
			}
			if got.Answer != a+b {
				t.Errorf("GenerateCaptcha() Answer = %d, want %d", got.Answer, a+b)
			}
		})
	}
}
