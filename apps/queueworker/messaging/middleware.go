package messaging

import (
	"context"
	"crypto/rand"
	"math/big"

	"github.com/pentops/log.go/log"
	"github.com/pentops/o5-messaging/gen/o5/messaging/v1/messaging_pb"
)

type ResendHandler struct {
	resendChance int
	handler      Handler
}

func NewResendHandler(handler Handler, resendChance int) *ResendHandler {
	return &ResendHandler{
		handler:      handler,
		resendChance: resendChance,
	}
}

func (ww *ResendHandler) HandleMessage(ctx context.Context, msg *messaging_pb.Message) error {
	if err := ww.handler.HandleMessage(ctx, msg); err != nil {
		return err
	}
	if randomlySelected(ctx, ww.resendChance) {
		if err := ww.handler.HandleMessage(ctx, msg); err != nil {
			return err
		}
	}
	return nil
}

// Is this message is randomly selected based on percent received?
func randomlySelected(ctx context.Context, pct int) bool {
	if pct == 0 {
		return false
	}

	if pct == 100 {
		return true
	}

	if pct > 100 || pct < 0 {
		log.Infof(ctx, "Received invalid percent for randomly selecting a message: %v", pct)
		return false
	}

	r, err := rand.Int(rand.Reader, big.NewInt(100))
	if err != nil {
		log.WithError(ctx, err).Error("couldn't generate random number for selecting message")
		return false
	}

	if r.Int64() <= big.NewInt(int64(pct)).Int64() {
		log.Infof(ctx, "Message randomly selected: rand of %v and percent of %v", r.Int64(), pct)
		return true
	}
	return false
}
