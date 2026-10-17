CREATE TABLE staff_messages (
    id BIGSERIAL PRIMARY KEY,
    message_id TEXT NOT NULL UNIQUE,
    tenant_public_id TEXT NOT NULL,
    event_id TEXT NOT NULL,
    sender_user_id TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('staff_message', 'guidance_info')),
    audience TEXT NOT NULL CHECK (audience IN ('staff', 'guest', 'both')),
    body TEXT NOT NULL CHECK (body <> ''),
    related_route_ids JSONB NOT NULL DEFAULT '[]',
    sent_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    CHECK (kind = 'guidance_info' OR related_route_ids = '[]'),
    CHECK (kind = 'staff_message' OR audience = 'staff')
);
CREATE INDEX staff_messages_event_id_sent_at_idx ON staff_messages (event_id, sent_at);

CREATE TABLE reassignments (
    id BIGSERIAL PRIMARY KEY,
    reassignment_id TEXT NOT NULL UNIQUE,
    tenant_public_id TEXT NOT NULL,
    event_id TEXT NOT NULL,
    staff_user_id TEXT NOT NULL,
    target_kind TEXT NOT NULL CHECK (target_kind IN ('observation_point', 'gate_point', 'graph_point', 'graph_route')),
    target_id TEXT NOT NULL CHECK (target_id <> ''),
    route_position DOUBLE PRECISION CHECK (route_position BETWEEN 0 AND 1),
    status TEXT NOT NULL CHECK (status IN ('directed', 'applied')),
    directed_at TIMESTAMPTZ NOT NULL,
    CHECK (target_kind = 'graph_route' OR route_position IS NULL)
);
CREATE INDEX reassignments_event_staff_directed_at_idx
    ON reassignments (tenant_public_id, event_id, staff_user_id, directed_at DESC, id DESC);

CREATE TABLE connection_states (
    tenant_public_id TEXT NOT NULL,
    event_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    device_id TEXT NOT NULL,
    connected BOOLEAN NOT NULL,
    at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_public_id, event_id, user_id, device_id)
);

CREATE TABLE feedback_values (
    id BIGSERIAL PRIMARY KEY,
    event_id TEXT NOT NULL,
    computed_at TIMESTAMPTZ NOT NULL,
    attribute_tag TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind <> ''),
    value DOUBLE PRECISION NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (event_id, computed_at, attribute_tag, kind)
);
