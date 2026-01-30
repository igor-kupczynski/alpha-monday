CREATE TABLE batches (
  id uuid PRIMARY KEY,
  created_at timestamptz NOT NULL DEFAULT now(),
  run_date date NOT NULL,
  benchmark_symbol text NOT NULL DEFAULT 'SPY',
  benchmark_initial_price numeric NOT NULL,
  status text NOT NULL CONSTRAINT batches_status_check CHECK (status IN ('active', 'completed', 'failed')),
  CONSTRAINT batches_run_date_unique UNIQUE (run_date)
);
