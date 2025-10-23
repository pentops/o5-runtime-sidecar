package queueworker

import (
	"context"
	"fmt"

	"github.com/pentops/o5-runtime-sidecar/apps/queueworker/sqslink"
	"github.com/pentops/o5-runtime-sidecar/sidecar"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type WorkerConfig struct {
	SQSURL        string `env:"SQS_URL" default:""`
	ResendChance  int    `env:"RESEND_CHANCE" required:"false"`
	NoDeadLetters bool   `env:"NO_DEADLETTERS" default:"false"`
}

type App struct {
	queueWorker *sqslink.Worker
	router      *sqslink.Router
}

type Publisher interface {
	sqslink.Publisher
}

func NewApp(config WorkerConfig, info sidecar.AppInfo, publisher sqslink.Publisher, sqs sqslink.SQSAPI) (*App, error) {
	var dlh sqslink.DeadLetterHandler
	if !config.NoDeadLetters {
		if publisher == nil {
			return nil, fmt.Errorf("SQS queue worker requires a sender for dead letters (set EVENTBRIDGE_ARN)")
		}

		dlh = sqslink.NewO5MessageDeadLetterHandler(publisher, info)
	}

	router := sqslink.NewRouter()

	ww := sqslink.NewWorker(sqs, config.SQSURL, dlh, config.ResendChance, router)
	return &App{
		router:      router,
		queueWorker: ww,
	}, nil

}

func (app *App) Run(ctx context.Context) error {
	return app.queueWorker.Run(ctx)
}

func (app *App) RegisterService(ctx context.Context, service protoreflect.ServiceDescriptor, invoker sqslink.AppLink) error {
	return app.router.RegisterService(ctx, service, invoker)
}
