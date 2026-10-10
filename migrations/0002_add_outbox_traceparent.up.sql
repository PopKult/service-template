-- Additive and nullable: a previous release's code (which neither writes nor
-- reads this column) keeps working during a canary rollout, and rows written
-- before this migration simply have no trace to continue.
ALTER TABLE outbox_events ADD COLUMN traceparent TEXT;
