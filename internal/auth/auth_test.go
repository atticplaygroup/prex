package auth_test

import (
	"encoding/base64"
	"log"
	"time"

	"github.com/atticplaygroup/prex/internal/auth"
	"github.com/atticplaygroup/prex/internal/config"
	"github.com/block-vision/sui-go-sdk/models"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type mockClock struct {
	MockNow time.Time
}

func (c mockClock) Now() time.Time { return c.MockNow }

var _ = Describe("Authentication", Label("auth"), func() {
	auth1, err := auth.NewAuth(config.Config{
		JwtSecret:          "0x1234",
		MessageAuthTimeout: 10 * time.Second,
		SessionTimeout:     24 * time.Hour,
	})
	if err != nil {
		log.Fatalf("failed to initialize auth: %v", err)
	}
	auth1.Clock = mockClock{MockNow: time.Unix(1700000000, 0)}
	walletAddress := "0xc228a949decc98affe62522cf3f56db12686d068fab82fab605c241cafe5197c"
	var actualChallengeBytes *[32]byte
	It("should generate challenge", func() {
		actualChallengeBytes, err = auth1.GetChallenge(walletAddress, auth1.Clock.Now())
		Expect(err).To(BeNil())
		actualChallenge := base64.StdEncoding.EncodeToString(actualChallengeBytes[:])
		Expect(actualChallenge).To(Equal("TVecreI73Eoq25g1Cy3pTBErsL7lCfVkK0xJrzgei28="))
	})

	It("should verify correct personal message signature by chain address", func() {
		sender, pass, err := models.VerifyPersonalMessage(
			string((*actualChallengeBytes)[:]),
			"AH1itCc8k3ZKMI4XRyPjftCaTjxWs+sko1xe1ZQ569VyGOueIekgvfoaRNm5urIKHd9gf9rVych7xBQycV88Pw4+9VRkoxpm+CpWKk6FUF9WOnno84czkMnCUR/W1scgkw==",
		)
		Expect(pass).To(BeTrue())
		Expect(err).To(BeNil())
		address := sender
		Expect(address).To(Equal(walletAddress))
	})

	It("should verify offline login payload and signature", func() {
		startTime := auth1.Clock.Now()
		walletAddress := "0xc228a949decc98affe62522cf3f56db12686d068fab82fab605c241cafe5197c"
		challengeBytes, err := auth1.GetChallenge(walletAddress, startTime)
		Expect(err).To(BeNil())

		authMessage, err := auth1.EncodeMessage(
			startTime,
			(*challengeBytes)[:],
			walletAddress,
		)
		Expect(err).To(BeNil())
		err = auth1.VerifySuiAuthMessagePayload(authMessage)
		Expect(err).To(BeNil())
	})
})
