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
	"github.com/pentops/o5-runtime-sidecar/awsmsg"
	"github.com/pentops/sqrlx.go/sqrlx"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/pentops/o5-go/messaging/v1/messaging_pb"
)

type Batcher interface {
	PublishBatch(ctx context.Context, messages []*messaging_pb.Message) ([]string, error)
}

type Listener struct {
	dbURL     string
	source    awsmsg.SourceConfig
	publisher Batcher
}

func NewListener(dbURL string, publisher Batcher, sourceConfig awsmsg.SourceConfig) (*Listener, error) {
	_, err := pq.ParseURL(dbURL)
	if err != nil {
		// the URL can contain secrets, so we don't want to log it... but it
		// does make debugging difficult.
		return nil, fmt.Errorf("parsing postgres URL: %w", err)
	}
	return &Listener{
		dbURL:     dbURL,
		source:    sourceConfig,
		publisher: publisher,
	}, nil
}

func (ll *Listener) Listen(ctx context.Context) error {

	db, err := sql.Open("postgres", ll.dbURL)
	if err != nil {
		return err
	}

	pqListener := pq.NewListener(ll.dbURL, time.Second*1, time.Second*10, func(le pq.ListenerEventType, err error) {
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

	runner := looper{
		db:       dbWrapped,
		listener: pqListener,
	}

	if err := pqListener.Listen("outboxmessage"); err != nil {
		return err
	}

	if err := runner.loopUntilEmpty(ctx, ll.rowsCallback); err != nil {
		return err
	}

	for range pqListener.NotificationChannel() {
		err := runner.loopUntilEmpty(ctx, ll.rowsCallback)
		if err != nil {
			return err
		}
	}

	return nil
}

func (ll *Listener) rowsCallback(ctx context.Context, rows []outboxRow) ([]string, error) {
	msgs := make([]*messaging_pb.Message, len(rows))
	for idx, row := range rows {
		msg, err := parseOutboxMessage(row, ll.source)
		if err != nil {
			return nil, err
		}

		msgs[idx] = msg
	}

	return ll.publisher.PublishBatch(ctx, msgs)
}

func parseOutboxMessage(row outboxRow, source awsmsg.SourceConfig) (*messaging_pb.Message, error) {

	headers, err := url.ParseQuery(row.header)
	if err != nil {
		return nil, fmt.Errorf("error parsing headers from outbox message: %w", err)
	}
	simpleHeaders := map[string]string{}
	for k, v := range headers {
		simpleHeaders[k] = v[0]
	}

	protoMessageName, ok := simpleHeaders["grpc-message"]
	if !ok || protoMessageName == "" {
		return nil, fmt.Errorf("grpc-message header missing from outbox message")
	}
	delete(simpleHeaders, "grpc-message")

	protoServiceName, ok := simpleHeaders["grpc-service"]
	if !ok || protoServiceName == "" {
		return nil, fmt.Errorf("grpc-service header missing from outbox message")
	}
	delete(simpleHeaders, "grpc-service")

	var protoMethodName string

	if strings.HasPrefix(protoServiceName, "/") {
		parts := strings.Split(protoServiceName, "/")
		if len(parts) != 3 {
			return nil, fmt.Errorf("invalid service name: %s", protoServiceName)
		}
		protoServiceName = parts[1]
		protoMethodName = parts[2]
	} else {
		protoMethodName, ok = simpleHeaders["grpc-method"]
		if !ok || protoMethodName == "" {
			return nil, fmt.Errorf("grpc-method header missing from outbox message and grpc-service isn't a full descriptor")
		}

	}

	msg := &messaging_pb.Message{
		MessageId:        row.id,
		DestinationTopic: row.destination,
		GrpcMethod:       protoMethodName,
		GrpcService:      protoServiceName,
		SourceApp:        source.SourceApp,
		SourceEnv:        source.SourceEnv,
		Headers:          simpleHeaders,
		Body: &messaging_pb.Any{
			TypeUrl: fmt.Sprintf("type.googleapis.com/%s", protoMessageName),
			Value:   row.message,
		},
		Timestamp: timestamppb.Now(),
	}

	contentEncoding, ok := simpleHeaders["wire-encoding"]
	if ok {
		enc, ok := messaging_pb.WireEncoding_value_either[contentEncoding]
		if ok {
			msg.Body.Encoding = messaging_pb.WireEncoding(enc)
		}
	}

	replyReply, ok := simpleHeaders["o5-reply-reply-to"]
	if ok {
		msg.Extension = &messaging_pb.Message_Reply_{
			Reply: &messaging_pb.Message_Reply{
				ReplyTo: replyReply,
			},
		}
		delete(msg.Headers, "o5-reply-reply-to")
	}

	requestReplyTo, ok := simpleHeaders["o5-reply-to"]
	if ok {
		msg.Extension = &messaging_pb.Message_Request_{
			Request: &messaging_pb.Message_Request{
				ReplyTo: requestReplyTo,
			},
		}
		delete(msg.Headers, "o5-reply-to")
	}

	return msg, nil
}

type looper struct {
	listener *pq.Listener
	db       *sqrlx.Wrapper
}

type pageCallback func(ctx context.Context, rows []outboxRow) ([]string, error)

func (ll looper) loopUntilEmpty(ctx context.Context, callback pageCallback) error {
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

type outboxRow struct {
	id          string
	destination string
	message     []byte
	header      string
}

func (ll looper) doPage(ctx context.Context, callback pageCallback) (int, error) {
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

		msgRows := []outboxRow{}

		for rows.Next() {
			count++

			var row outboxRow

			if err := rows.Scan(
				&row.id,
				&row.destination,
				&row.message,
				&row.header,
			); err != nil {
				return fmt.Errorf("error scanning outbox row: %w", err)
			}

			msgRows = append(msgRows, row)

		}

		if err := rows.Err(); err != nil {
			return fmt.Errorf("error in outbox rows: %w", err)
		}

		if count == 0 {
			return nil
		}

		// NOTE: Error handling from here is out of usual order.

		successIDs, sendError := callback(ctx, msgRows)

		res, deleteError := tx.Delete(ctx, sq.
			Delete("outbox").
			Where("id = ANY(?)", pq.Array(successIDs)))

		if sendError != nil {
			return fmt.Errorf("error sending batch of outbox messages: %w", sendError)
		}

		if deleteError != nil {
			return fmt.Errorf("error deleting sent outbox messages: %w", deleteError)
		}
		rowsAffected, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("error getting rows affected: %w", err)
		}
		if rowsAffected != int64(len(successIDs)) {
			return fmt.Errorf("expected to delete %d rows, but deleted %d", len(successIDs), rowsAffected)
		}

		return nil
	})
	return count, err
}
