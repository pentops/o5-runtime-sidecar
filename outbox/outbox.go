package outbox

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"time"

	sq "github.com/elgris/sqrl"
	"github.com/lib/pq"
	"github.com/pentops/log.go/log"
	"github.com/pentops/sqrlx.go/sqrlx"
)

type Message struct {
	ID                string            `json:"id"`
	Message           []byte            `json:"body"`
	Destination       string            `json:"topic-name"`
	Headers           map[string]string `json:"headers"`
	SourceEnvironment string            `json:"source-environment"`
	ContentType       string            `json:"content-type"`

	GrpcService string  `json:"grpc-service"`
	GrpcMethod  string  `json:"grpc-method"`
	GrpcMessage string  `json:"grpc-message"`
	ReplyDest   *string `json:"reply-dest,omitempty"`
}

func (msg *Message) expandAndValidate() error {
	for k, v := range msg.Headers {
		v := v
		if k == "grpc-service" {
			msg.GrpcService = v
			delete(msg.Headers, k)
			continue
		}
		if k == "grpc-method" {
			msg.GrpcMethod = v
			delete(msg.Headers, k)
			continue
		}
		if k == "grpc-message" {
			msg.GrpcMessage = v
			delete(msg.Headers, k)
			continue
		}
		if k == "content-type" {
			msg.ContentType = v
			delete(msg.Headers, k)
			continue
		}
		if k == "o5-reply-reply-to" {
			msg.ReplyDest = &v
			delete(msg.Headers, k)
			continue
		}
	}

	if msg.GrpcMessage == "" {
		return fmt.Errorf("grpc-message header missing from outbox message")
	}

	if msg.GrpcService == "" {
		return fmt.Errorf("grpc-service header missing from outbox message")
	}

	parts := strings.Split(msg.GrpcService, "/")
	if len(parts) == 2 && parts[0] == "" {
		// '/package.service'
		parts = []string{parts[1]}
	}
	if len(parts) == 1 {
		// 'package.service'
		if msg.GrpcMethod == "" {
			return fmt.Errorf("grpc-service header should be /package.service/method, or grpc-method should be set. %s", msg.GrpcService)
		}
	} else {
		// '/package.service/method'
		if len(parts) != 3 || parts[0] != "" {
			return fmt.Errorf("grpc-service header has wrong format, should be /package.service/method, or just package.service with grpc-method set : %s", msg.GrpcService)
		}
		if msg.GrpcMethod == "" {
			msg.GrpcMethod = parts[2]
		} else if msg.GrpcMethod != parts[2] {
			return fmt.Errorf("grpc-service header specified as /package.service/method, but grpc-method did not match: %s %s", msg.GrpcService, msg.GrpcMethod)
		}

		msg.GrpcService = parts[1]
	}

	return nil
}

type Batcher interface {
	SendMultiBatch(ctx context.Context, messages []*Message) ([]string, error)
}

func Listen(ctx context.Context, url string, callback Batcher) error {
	_, err := pq.ParseURL(url)
	if err != nil {
		// the URL can contain secrets, so we don't want to log it... but it
		// does make debugging difficult.
		return fmt.Errorf("parsing postgres URL: %w", err)
	}

	db, err := sql.Open("postgres", url)
	if err != nil {
		return err
	}

	pqListener := pq.NewListener(url, time.Second*1, time.Second*10, func(le pq.ListenerEventType, err error) {
		if err != nil {
			log.WithError(ctx, err).Error("error in listener")
		}
	})
	closed := false
	go func() {
		<-ctx.Done()
		closed = true
		pqListener.Close()
	}()
	for {
		if closed {
			return nil
		}
		if err := pqListener.Ping(); err != nil {
			log.WithError(ctx, err).Error("pinging Listener PG")
			time.Sleep(time.Second)
			continue
		}
		log.Info(ctx, "pinging Listener PG OK")
		break
	}

	dbWrapped, err := sqrlx.New(db, sq.Dollar)
	if err != nil {
		return err
	}

	ll := listener{
		db:       dbWrapped,
		listener: pqListener,
	}

	if err := pqListener.Listen("outboxmessage"); err != nil {
		return err
	}

	if err := ll.loopUntilEmpty(ctx, callback); err != nil {
		return err
	}

	for range pqListener.NotificationChannel() {
		err := ll.loopUntilEmpty(ctx, callback)
		if err != nil {
			return err
		}
	}

	return nil
}

type listener struct {
	listener *pq.Listener
	db       *sqrlx.Wrapper
}

func (ll listener) loopUntilEmpty(ctx context.Context, callback Batcher) error {
	for {
		count, err := ll.doPage(ctx, callback)
		if err != nil {
			return fmt.Errorf("error doing page of messages: %w", err)
		}
		if count == 0 {
			return nil
		}
	}
}

func (ll listener) doPage(ctx context.Context, callback Batcher) (int, error) {
	qq := sq.Select(
		"id",
		"destination",
		"message",
		"headers",
	).From("outbox").
		OrderBy("destination ASC").
		Limit(100).
		Suffix("FOR UPDATE SKIP LOCKED")

	var count int

	err := ll.db.Transact(ctx, &sqrlx.TxOptions{
		Isolation: sql.LevelDefault,
		Retryable: true,
		ReadOnly:  false,
	}, func(ctx context.Context, tx sqrlx.Transaction) error {
		count = 0
		rows, err := tx.Select(ctx, qq)
		if err != nil {
			return fmt.Errorf("error selecting outbox messages: %w", err)
		}

		defer rows.Close()

		messages := []*Message{}

		for rows.Next() {
			count++
			var id, destination string
			var message []byte
			var headerString string

			if err := rows.Scan(
				&id,
				&destination,
				&message,
				&headerString,
			); err != nil {
				return fmt.Errorf("error scanning outbox row: %w", err)
			}

			headers, err := url.ParseQuery(headerString)
			if err != nil {
				return fmt.Errorf("error parsing headers from outbox message: %w", err)
			}
			simpleHeaders := map[string]string{}
			for k, v := range headers {
				simpleHeaders[k] = v[0]
			}

			msg := &Message{
				ID:          id,
				Message:     message,
				Headers:     simpleHeaders,
				Destination: destination,
			}

			if err := msg.expandAndValidate(); err != nil {
				return err
			}

			messages = append(messages, msg)
		}

		if err := rows.Err(); err != nil {
			return fmt.Errorf("error in outbox rows: %w", err)
		}

		// NOTE: Error handling from here is out of usual order.

		successIDs, sendError := callback.SendMultiBatch(ctx, messages)

		_, txError := tx.Exec(ctx, sq.
			Delete("outbox").
			Where("id = ANY(?)", pq.Array(successIDs)))

		if sendError != nil {
			return fmt.Errorf("error sending batch of outbox messages: %w", err)
		}

		if txError != nil {
			return fmt.Errorf("error deleting sent outbox messages: %w", err)
		}

		return nil
	})
	return count, err
}
