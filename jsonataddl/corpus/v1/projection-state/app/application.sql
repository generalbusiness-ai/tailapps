CREATE EVENT otel_event (
  key TEXT NOT NULL,
  amount INTEGER NOT NULL,
  retire BOOLEAN
);

CREATE TABLE ledger (
  key TEXT NOT NULL,
  balance INTEGER NOT NULL,
  PRIMARY KEY (key)
);

CREATE TABLE audit_marks (
  key TEXT NOT NULL,
  mark INTEGER NOT NULL,
  PRIMARY KEY (key, mark)
);

CREATE TABLE shadow_notes (
  key TEXT NOT NULL,
  note TEXT NOT NULL,
  PRIMARY KEY (key)
);

CREATE VIEW ledger_positive AS
  SELECT key, balance FROM ledger WHERE balance > 0;

CREATE NORMALIZER normalize ON otlp_record
USING 'folds/normalize.jsonata'
EMITS otel_event;

CREATE FOLD settle ON otel_event
READ prior OPTIONAL ONE AS
  SELECT key, balance
  FROM ledger
  WHERE key = :event.key
READ marks MANY LIMIT 10 AS
  SELECT key, mark
  FROM audit_marks
  WHERE key = :event.key
  ORDER BY key, mark
READ positive OPTIONAL ONE AS
  SELECT key, balance
  FROM ledger_positive
  WHERE key = :event.key
USING 'folds/settle.jsonata'
WRITES ledger, audit_marks;

CREATE FOLD shadow ON otel_event
USING 'folds/shadow.jsonata'
WRITES shadow_notes;

CREATE EXPORT ledger AS
  SELECT key, balance FROM ledger;
