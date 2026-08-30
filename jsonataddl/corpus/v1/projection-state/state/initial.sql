INSERT INTO ledger (key, balance) VALUES ('alpha', 40);
INSERT INTO audit_marks (key, mark) VALUES ('alpha', 1);
INSERT INTO audit_marks (key, mark) VALUES ('alpha', 2);
INSERT INTO shadow_notes (key, note) VALUES ('alpha', 'private');
CREATE TABLE host_secrets (name TEXT PRIMARY KEY, secret TEXT NOT NULL);
INSERT INTO host_secrets (name, secret) VALUES ('token', 'hunter2');
