CREATE TABLE checkpoints (
  id uuid PRIMARY KEY,
  batch_id uuid NOT NULL CONSTRAINT checkpoints_batch_fk REFERENCES batches(id),
  checkpoint_date date NOT NULL,
  status text NOT NULL CONSTRAINT checkpoints_status_check CHECK (status IN ('computed', 'skipped')),
  benchmark_price numeric,
  benchmark_return_pct numeric,
  CONSTRAINT checkpoints_batch_date_unique UNIQUE (batch_id, checkpoint_date)
);

CREATE INDEX checkpoints_batch_id_idx ON checkpoints (batch_id);
