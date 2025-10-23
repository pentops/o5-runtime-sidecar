package queueworker

import (
	"context"
	"fmt"

	"github.com/pentops/o5-runtime-sidecar/adapters/sqsmsg"
	"github.com/pentops/o5-runtime-sidecar/apps/queueworker/messaging"
	"github.com/pentops/o5-runtime-sidecar/sidecar"
)

type WorkerConfig struct {
	SQSURL        string `env:"SQS_URL" default:""`
	ResendChance  int    `env:"RESEND_CHANCE" required:"false"`
	NoDeadLetters bool   `env:"NO_DEADLETTERS" default:"false"`
}

type App struct {
	queueWorker *sqsmsg.Worker
}

type Publisher interface {
	messaging.Publisher
}

func NewApp(config WorkerConfig, info sidecar.AppInfo, publisher messaging.Publisher, sqs sqsmsg.SQSAPI, handler messaging.Handler) (*App, error) {
	var dlh messaging.DeadLetterHandler
	if !config.NoDeadLetters {
		if publisher == nil {
			return nil, fmt.Errorf("SQS queue worker requires a sender for dead letters (set EVENTBRIDGE_ARN)")
		}

		dlh = messaging.NewO5MessageDeadLetterHandler(publisher, info)
	}

	if config.ResendChance > 0 {
		handler = messaging.NewResendHandler(handler, config.ResendChance)
	}

	ww := sqsmsg.NewWorker(sqs, config.SQSURL, dlh, handler)
	return &App{
		queueWorker: ww,
	}, nil

}

func (app *App) Run(ctx context.Context) error {
	return app.queueWorker.Run(ctx)
}
