
-- +goose Up
CREATE TABLE IF NOT EXISTS outbox (
	id uuid PRIMARY KEY,
	data jsonb NOT NULL,
	headers text NOT NULL,
	send_after timestamptz NOT NULL
);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION outbox_notify()
  RETURNS TRIGGER AS $$ DECLARE
BEGIN
  NOTIFY outboxmessage;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE OR REPLACE TRIGGER outbox_notify
AFTER INSERT ON outbox
EXECUTE PROCEDURE outbox_notify();

-- +goose Down

DROP TRIGGER outbox_notify ON outbox;
DROP FUNCTION outbox_notify;
DROP TABLE outbox;
