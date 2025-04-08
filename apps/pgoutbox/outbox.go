package pgoutbox

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pentops/log.go/log"

	"github.com/pentops/o5-messaging/gen/o5/messaging/v1/messaging_pb"
)

type Batcher interface {
	PublishBatch(ctx context.Context, messages []*messaging_pb.Message) ([]string, error)
}

type Parser interface {
	ParseMessage(id string, data []byte) (*messaging_pb.Message, error)
}

type pgConnector interface {
	DSN(ctx context.Context) (string, error)
}

type Listener struct {
	connector pgConnector
	publisher Batcher
	parser    Parser
}

func NewListener(connector pgConnector, publisher Batcher, parser Parser) (*Listener, error) {
	return &Listener{
		connector: connector,
		publisher: publisher,
		parser:    parser,
	}, nil
}

func (ll *Listener) connectAndListen(ctx context.Context) (*pgx.Conn, error) {
	dialContext, cancel := context.WithTimeout(ctx, time.Second*5)
	defer cancel()

	dsn, err := ll.connector.DSN(dialContext)
	if err != nil {
		return nil, fmt.Errorf("getting connection DSN: %w", err)
	}

	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("connecting to PG: %w", err)
	}

	for {
		if err := conn.Ping(ctx); err != nil {
			log.WithError(ctx, err).Warn("pinging Listener PG")
			time.Sleep(time.Second)
			continue
		}

		log.Info(ctx, "pinging Listener PG OK")
		break
	}

	if _, err := conn.Exec(ctx, "LISTEN outboxmessage"); err != nil {
		return nil, err
	}

	return conn, nil
}

func (ll *Listener) Listen(ctx context.Context) error {
	conn, err := ll.connectAndListen(ctx)
	if err != nil {
		return err
	}

	err = ll.loopUntilEmpty(ctx, conn)
	if err != nil {
		return err
	}

	for {
		log.Debug(ctx, "waiting for notification")

		_, err := conn.WaitForNotification(ctx)
		if err != nil {
			log.WithError(ctx, err).Warn("listener error, reconnecting")
			conn.Close(ctx)

			newConn, err := ll.connectAndListen(ctx)
			if err != nil {
				return err
			}

			conn = newConn
			log.Info(ctx, "reconnected to PG")
		} else {
			log.Debug(ctx, "received notification")
		}

		err = ll.loopUntilEmpty(ctx, conn)
		if err != nil {
			return err
		}
	}

}

func (ll *Listener) rowsCallback(ctx context.Context, rows []outboxRow) ([]string, error) {
	msgs := make([]*messaging_pb.Message, len(rows))
	for idx, row := range rows {
		msg, err := ll.parseOutboxMessage(row)
		if err != nil {
			return nil, err
		}

		msgs[idx] = msg
	}

	return ll.publisher.PublishBatch(ctx, msgs)
}

func (ll *Listener) parseOutboxMessage(row outboxRow) (*messaging_pb.Message, error) {
	msg, err := ll.parser.ParseMessage(row.id, row.message)
	if err != nil {
		return nil, fmt.Errorf("error parsing outbox message: %w", err)
	}

	return msg, nil
}

type pageCallback func(ctx context.Context, rows []outboxRow) ([]string, error)

func (ll *Listener) loopUntilEmpty(ctx context.Context, conn *pgx.Conn) error {
	log.Debug(ctx, "loopUntilEmpty")
	for {
		log.Debug(ctx, "doPage")

		count, err := ll.doPage(ctx, conn)
		if err != nil {
			log.WithError(ctx, err).Error("Error running message page")
			return fmt.Errorf("error doing page of messages: %w", err)
		}

		log.WithField(ctx, "count", count).Debug("didPage")

		if count == 0 {
			return nil
		}
	}
}

type outboxRow struct {
	id      string
	message []byte
}

func (ll *Listener) doPage(ctx context.Context, conn *pgx.Conn) (int, error) {
	var count int

	tx, err := conn.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.ReadCommitted,
	})
	if err != nil {
		return 0, fmt.Errorf("error beginning transaction: %w", err)
	}

	var sendError error
	err = func() error {
		count = 0
		rows, err := tx.Query(ctx, "SELECT id, data FROM outbox LIMIT 10 FOR UPDATE SKIP LOCKED")
		if err != nil {
			return fmt.Errorf("error selecting outbox messages: %w", err)
		}

		defer rows.Close()

		msgRows := []outboxRow{}

		for rows.Next() {
			count++

			var row outboxRow

			err := rows.Scan(&row.id, &row.message)
			if err != nil {
				return fmt.Errorf("error scanning outbox row: %w", err)
			}

			msgRows = append(msgRows, row)
		}

		log.WithField(ctx, "count", count).Debug("got outbox messages")

		err = rows.Err()
		if err != nil {
			return fmt.Errorf("error in outbox rows: %w", err)
		}

		if count == 0 {
			return nil
		}

		var successIDs []string

		msgs := make([]*messaging_pb.Message, len(msgRows))
		for idx, row := range msgRows {
			msg, err := ll.parseOutboxMessage(row)
			if err != nil {
				return err
			}

			msgs[idx] = msg
		}

		successIDs, sendError = ll.publisher.PublishBatch(ctx, msgs)
		// NOTE: sendError is handled at the end, the transaction should still be
		// committed.

		log.WithField(ctx, "successCount", len(successIDs)).Debug("published outbox messages")

		res, deleteError := tx.Exec(ctx, "DELETE FROM outbox WHERE id = ANY($1)", successIDs)
		if deleteError != nil {
			return fmt.Errorf("error deleting sent outbox messages: %w", deleteError)
		}

		rowsAffected := res.RowsAffected()
		if rowsAffected != int64(len(successIDs)) {
			return fmt.Errorf("expected to delete %d rows, but deleted %d", len(successIDs), rowsAffected)
		}

		return nil
	}()

	if err != nil {
		_ = tx.Rollback(ctx)
		return 0, err
	}

	err = tx.Commit(ctx)
	if err != nil {
		return 0, fmt.Errorf("error committing transaction: %w", err)
	}

	if sendError != nil {
		return 0, fmt.Errorf("error sending batch of outbox messages: %w", sendError)
	}

	return count, nil
}
