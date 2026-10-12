ALTER TABLE schema_migrations ADD COLUMN checksum TEXT NOT NULL DEFAULT '';
ALTER TABLE groceries ADD COLUMN unit TEXT NOT NULL DEFAULT '';
ALTER TABLE telegram_updates ADD COLUMN intake_seq INTEGER;
CREATE UNIQUE INDEX updates_intake ON telegram_updates(intake_seq);
CREATE TRIGGER groceries_quantity_insert BEFORE INSERT ON groceries
WHEN typeof(NEW.quantity) NOT IN ('real','integer') OR NEW.quantity<=0 OR NEW.quantity>1000000
BEGIN SELECT RAISE(ABORT,'invalid numeric quantity'); END;
CREATE TRIGGER groceries_quantity_update BEFORE UPDATE OF quantity ON groceries
WHEN typeof(NEW.quantity) NOT IN ('real','integer') OR NEW.quantity<=0 OR NEW.quantity>1000000
BEGIN SELECT RAISE(ABORT,'invalid numeric quantity'); END;
CREATE TABLE transport_state(key TEXT PRIMARY KEY, value TEXT NOT NULL) STRICT;
CREATE TABLE layout_cache(chat_id INTEGER PRIMARY KEY,revision INTEGER NOT NULL,fingerprint TEXT NOT NULL,ids_json TEXT NOT NULL) STRICT;
CREATE TABLE shopping_messages(chat_id INTEGER NOT NULL,message_id INTEGER NOT NULL,PRIMARY KEY(chat_id,message_id)) STRICT;
CREATE TABLE mutation_journal(id INTEGER PRIMARY KEY,chat_id INTEGER NOT NULL,user_id INTEGER NOT NULL,update_id INTEGER NOT NULL,before_json TEXT NOT NULL,after_json TEXT NOT NULL,undone_at TEXT,created_at TEXT NOT NULL) STRICT;
CREATE INDEX journal_user ON mutation_journal(chat_id,user_id,id DESC);
CREATE TABLE templates(chat_id INTEGER NOT NULL,name TEXT NOT NULL,items_json TEXT NOT NULL,PRIMARY KEY(chat_id,name)) STRICT;
CREATE TABLE aliases(chat_id INTEGER NOT NULL,alias TEXT NOT NULL,preferred_name TEXT NOT NULL,PRIMARY KEY(chat_id,alias)) STRICT;
CREATE TABLE llm_usage(id INTEGER PRIMARY KEY,request_key TEXT NOT NULL UNIQUE,created_at TEXT NOT NULL,state TEXT NOT NULL,prompt_tokens INTEGER NOT NULL DEFAULT 0,completion_tokens INTEGER NOT NULL DEFAULT 0,reasoning_tokens INTEGER NOT NULL DEFAULT 0,cost_micros INTEGER NOT NULL DEFAULT 0,reserved_micros INTEGER NOT NULL DEFAULT 0,latency_ms INTEGER NOT NULL DEFAULT 0) STRICT;
UPDATE settings SET value_json='4096' WHERE key='llm.max_output_tokens' AND value_json='800';
