package pkg

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

type Captcha struct {
	Question string
	Answer   int
}

func GenerateCaptcha() (Captcha, error) {
	n1, err := rand.Int(rand.Reader, big.NewInt(10))
	if err != nil {
		return Captcha{}, fmt.Errorf("failed to generate random number 1: %w", err)
	}
	n2, err := rand.Int(rand.Reader, big.NewInt(10))
	if err != nil {
		return Captcha{}, fmt.Errorf("failed to generate random number 2: %w", err)
	}

	a := int(n1.Int64()) + 1
	b := int(n2.Int64()) + 1
	return Captcha{
		Question: fmt.Sprintf("%d + %d", a, b),
		Answer:   a + b,
	}, nil
}
