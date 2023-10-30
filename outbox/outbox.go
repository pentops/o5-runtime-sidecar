package outbox

import (
	"context"
	"database/sql"
	"net/url"
	"time"

	sq "github.com/elgris/sqrl"
	"github.com/lib/pq"
	"github.com/pentops/log.go/log"
	"gopkg.daemonl.com/sqrlx"
)

type Message struct {
	ID      string
	Message []byte
	Headers map[string][]string
}

type Batcher interface {
	SendBatch(ctx context.Context, destination string, messages []*Message) error
}

func Listen(ctx context.Context, url string, callback Batcher) error {

	db, err := sql.Open("postgres", url)
	if err != nil {
		return err
	}

	pqListener := pq.NewListener(url, time.Second*1, time.Second*10, func(le pq.ListenerEventType, err error) {
		if err != nil {
			log.WithError(ctx, err).Error("error in listener")
		}
	})
	for {
		if err := pqListener.Ping(); err != nil {
			log.WithError(ctx, err).Error("pinging Listener PG")
			time.Sleep(time.Second)
			continue
		}
		log.Info(ctx, "pinging Listener PG OK")
		break
	}

	go func() {
		<-ctx.Done()
		pqListener.Close()
	}()

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
			return err
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
			return err
		}

		defer rows.Close()

		byDestination := map[string][]*Message{}
		successIDs := []string{}

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
				return err
			}

			headers, err := url.ParseQuery(headerString)
			if err != nil {
				return err
			}

			msg := &Message{
				ID:      id,
				Message: message,
				Headers: headers,
			}

			byDestination[destination] = append(byDestination[destination], msg)
		}

		if err := rows.Err(); err != nil {
			return err
		}

		for destination, messages := range byDestination {
			if err := callback.SendBatch(ctx, destination, messages); err != nil {
				return err
			}
			for _, msg := range messages {
				successIDs = append(successIDs, msg.ID)
			}
		}

		_, err = tx.Exec(ctx, sq.
			Delete("outbox").
			Where("id = ANY(?)", pq.Array(successIDs)))
		if err != nil {
			return err
		}

		return nil
	})
	return count, err
}
