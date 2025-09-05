CREATE TABLE IF NOT EXISTS todos (
  id UUID PRIMARY KEY,
  title TEXT NOT NULL,
  completed BOOLEAN NOT NULL DEFAULT FALSE,
  deleted BOOLEAN NOT NULL DEFAULT FALSE,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT now(),
  last_modified TIMESTAMP WITH TIME ZONE DEFAULT now()
);

CREATE TABLE IF NOT EXISTS events (
  id SERIAL PRIMARY KEY,
  aggregate_id UUID NOT NULL,
  event_id UUID,
  type TEXT NOT NULL,
  payload JSONB NOT NULL,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT now()
);

-- Unique constraint on event_id for idempotency (nullable)
CREATE UNIQUE INDEX IF NOT EXISTS idx_events_event_id ON events(event_id);

CREATE TABLE IF NOT EXISTS projection_metadata (
    id SERIAL PRIMARY KEY,
    last_processed_timestamp TIMESTAMP WITH TIME ZONE
);

-- Insert an initial row if it doesn't exist
INSERT INTO projection_metadata(last_processed_timestamp) VALUES('epoch') ON CONFLICT DO NOTHING;