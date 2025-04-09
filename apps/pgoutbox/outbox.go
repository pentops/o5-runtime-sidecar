package pgoutbox

import (
	"context"
	"errors"
	"fmt"
	"time"

	sq "github.com/elgris/sqrl"
	"github.com/jackc/pgx/v5"
	"github.com/pentops/log.go/log"
	"golang.org/x/sync/errgroup"

	"github.com/pentops/o5-messaging/gen/o5/messaging/v1/messaging_pb"
)

var SendErr = errors.New("error sending batch of outbox messages")

type Batcher interface {
	PublishBatch(ctx context.Context, messages []*messaging_pb.Message) ([]string, error)
}

type Parser interface {
	ParseMessage(id string, data []byte) (*messaging_pb.Message, error)
}

type pgConnector interface {
	DSN(ctx context.Context) (string, error)
}

type Outbox struct {
	connector pgConnector
	publisher Batcher
	parser    Parser
}

func NewOutbox(connector pgConnector, publisher Batcher, parser Parser) (*Outbox, error) {
	return &Outbox{
		connector: connector,
		publisher: publisher,
		parser:    parser,
	}, nil
}

func (o *Outbox) Run(ctx context.Context) error {
	group, ctx := errgroup.WithContext(ctx)

	group.Go(func() error {
		log.Info(ctx, "starting outbox listener")

		err := o.Listen(ctx)
		if err != nil {
			return fmt.Errorf("error listening for outbox messages: %w", err)
		}

		log.Info(ctx, "stopping outbox listener")

		return nil
	})

	return group.Wait()
}

func (o *Outbox) Listen(ctx context.Context) error {
	conn, err := o.connectAndListen(ctx)
	if err != nil {
		return err
	}

	err = o.loopUntilEmpty(ctx, conn)
	if err != nil {
		return err
	}

	for {
		log.Debug(ctx, "waiting for notification")

		_, err := conn.WaitForNotification(ctx)
		if err != nil {
			log.WithError(ctx, err).Warn("listener error, reconnecting")
			conn.Close(ctx)

			newConn, err := o.connectAndListen(ctx)
			if err != nil {
				return err
			}

			conn = newConn
			log.Info(ctx, "reconnected to PG")
		} else {
			log.Debug(ctx, "received notification")
		}

		err = o.loopUntilEmpty(ctx, conn)
		if err != nil {
			return err
		}
	}
}

func (o *Outbox) connectAndListen(ctx context.Context) (*pgx.Conn, error) {
	dialContext, cancel := context.WithTimeout(ctx, time.Second*5)
	defer cancel()

	dsn, err := o.connector.DSN(dialContext)
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

func (o *Outbox) loopUntilEmpty(ctx context.Context, conn *pgx.Conn) error {
	log.Debug(ctx, "loopUntilEmpty")
	for {
		log.Debug(ctx, "doPage")

		count, err := o.doPage(ctx, conn)
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

func (o *Outbox) doPage(ctx context.Context, conn *pgx.Conn) (int, error) {
	var count int

	tx, err := conn.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.ReadCommitted,
	})
	if err != nil {
		return 0, fmt.Errorf("error beginning transaction: %w", err)
	}

	bErr := o.doBatch(ctx, tx)
	if bErr != nil && !errors.Is(bErr, SendErr) {
		_ = tx.Rollback(ctx)
		return 0, bErr
	}

	err = tx.Commit(ctx)
	if err != nil {
		return 0, fmt.Errorf("error committing transaction: %w", err)
	}

	if errors.Is(bErr, SendErr) {
		return 0, fmt.Errorf("error sending batch of outbox messages: %w", bErr)
	}

	return count, nil
}

type outboxRow struct {
	id      string
	message []byte
}

func (o *Outbox) doBatch(ctx context.Context, tx pgx.Tx) error {
	s := sq.Select("id", "data").
		From("outbox").
		Limit(10).
		Suffix(" FOR UPDATE SKIP LOCKED").
		PlaceholderFormat(sq.Dollar)

	q, a, err := s.ToSql()
	if err != nil {
		return fmt.Errorf("error building outbox query: %w", err)
	}

	count := 0
	rows, err := tx.Query(ctx, q, a...)
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

	msgs := make([]*messaging_pb.Message, len(msgRows))
	for idx, row := range msgRows {
		msg, err := o.parser.ParseMessage(row.id, row.message)
		if err != nil {
			return fmt.Errorf("error parsing outbox message: %w", err)
		}

		msgs[idx] = msg
	}

	// NOTE: this err is handled at the end to allow adding deletion of successful messages to the tx
	successIDs, delayedErr := o.publisher.PublishBatch(ctx, msgs)

	log.WithField(ctx, "successCount", len(successIDs)).Debug("published outbox messages")

	res, err := tx.Exec(ctx, "DELETE FROM outbox WHERE id = ANY($1)", successIDs)
	if err != nil {
		return fmt.Errorf("error deleting sent outbox messages: %w", err)
	}

	rowsAffected := res.RowsAffected()
	if rowsAffected != int64(len(successIDs)) {
		return fmt.Errorf("expected to delete %d rows, but deleted %d", len(successIDs), rowsAffected)
	}

	if delayedErr != nil {
		// mark as a send error so the caller can decide whether to commit or not
		return fmt.Errorf("%w: %w", SendErr, delayedErr)
	}

	return nil
}
