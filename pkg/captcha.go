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

func GenerateCaptcha() Captcha {
	n1, _ := rand.Int(rand.Reader, big.NewInt(10))
	n2, _ := rand.Int(rand.Reader, big.NewInt(10))
	a := int(n1.Int64()) + 1
	b := int(n2.Int64()) + 1
	return Captcha{
		Question: fmt.Sprintf("%d + %d", a, b),
		Answer:   a + b,
	}
}
