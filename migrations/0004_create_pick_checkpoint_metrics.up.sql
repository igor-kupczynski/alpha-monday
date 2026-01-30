CREATE TABLE pick_checkpoint_metrics (
  id uuid PRIMARY KEY,
  checkpoint_id uuid NOT NULL CONSTRAINT pick_checkpoint_metrics_checkpoint_fk REFERENCES checkpoints(id),
  pick_id uuid NOT NULL CONSTRAINT pick_checkpoint_metrics_pick_fk REFERENCES picks(id),
  current_price numeric NOT NULL,
  absolute_return_pct numeric NOT NULL,
  vs_benchmark_pct numeric NOT NULL,
  CONSTRAINT pick_checkpoint_metrics_checkpoint_pick_unique UNIQUE (checkpoint_id, pick_id)
);

CREATE INDEX pick_checkpoint_metrics_checkpoint_id_idx ON pick_checkpoint_metrics (checkpoint_id);
CREATE INDEX pick_checkpoint_metrics_pick_id_idx ON pick_checkpoint_metrics (pick_id);
