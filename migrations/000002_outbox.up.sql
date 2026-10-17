CREATE TABLE outbox_messages (
    id BIGSERIAL PRIMARY KEY,
    topic TEXT NOT NULL CHECK (topic IN ('guest-messages', 'realtime-delivery')),
    ordering_key TEXT NOT NULL CHECK (ordering_key <> ''),
    payload JSONB NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'sent', 'abandoned')),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX outbox_messages_pending_idx ON outbox_messages (id) WHERE status = 'pending';
