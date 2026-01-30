CREATE TABLE picks (
  id uuid PRIMARY KEY,
  batch_id uuid NOT NULL CONSTRAINT picks_batch_fk REFERENCES batches(id),
  ticker text NOT NULL,
  action text NOT NULL CONSTRAINT picks_action_check CHECK (action IN ('BUY', 'SELL')),
  reasoning text NOT NULL,
  initial_price numeric NOT NULL,
  CONSTRAINT picks_batch_ticker_unique UNIQUE (batch_id, ticker)
);

CREATE INDEX picks_batch_id_idx ON picks (batch_id);
