ALTER TABLE outbox_messages ALTER COLUMN payload TYPE BYTEA USING convert_to(payload::text, 'UTF8');
