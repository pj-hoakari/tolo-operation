ALTER TABLE outbox_messages ALTER COLUMN payload TYPE JSONB USING convert_from(payload, 'UTF8')::jsonb;
