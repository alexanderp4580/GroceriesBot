CREATE TABLE IF NOT EXISTS schema_migrations (
  version INTEGER PRIMARY KEY,
  applied_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS source_snapshots (
  id INTEGER PRIMARY KEY,
  digest TEXT NOT NULL UNIQUE,
  fetched_at TEXT NOT NULL,
  payload_zlib BLOB NOT NULL
);
CREATE TABLE IF NOT EXISTS source_sheets (
  id INTEGER PRIMARY KEY,
  snapshot_id INTEGER NOT NULL REFERENCES source_snapshots(id),
  spreadsheet_id TEXT NOT NULL,
  sheet_id INTEGER NOT NULL,
  title TEXT NOT NULL,
  locale TEXT NOT NULL,
  time_zone TEXT NOT NULL,
  grid_rows INTEGER NOT NULL,
  grid_columns INTEGER NOT NULL,
  UNIQUE(snapshot_id, spreadsheet_id, sheet_id)
);
CREATE TABLE IF NOT EXISTS source_rows (
  id INTEGER PRIMARY KEY,
  source_sheet_id INTEGER NOT NULL REFERENCES source_sheets(id),
  row_number INTEGER NOT NULL,
  cells_json TEXT NOT NULL CHECK(json_valid(cells_json)),
  UNIQUE(source_sheet_id, row_number)
);
CREATE TABLE IF NOT EXISTS import_issues (
  id INTEGER PRIMARY KEY,
  source_row_id INTEGER NOT NULL REFERENCES source_rows(id),
  code TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS groceries (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  chat_id INTEGER NOT NULL,
  legacy_id TEXT,
  name TEXT NOT NULL CHECK(length(trim(name)) > 0),
  category TEXT NOT NULL,
  quantity REAL NOT NULL CHECK(quantity > 0),
  added_at TEXT NOT NULL,
  purchased_at TEXT,
  deleted_at TEXT,
  version INTEGER NOT NULL DEFAULT 1,
  source_row_id INTEGER UNIQUE REFERENCES source_rows(id)
);
CREATE INDEX IF NOT EXISTS groceries_pending ON groceries(chat_id,category,id)
  WHERE purchased_at IS NULL AND deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS groceries_history ON groceries(chat_id,name,purchased_at)
  WHERE deleted_at IS NULL;
CREATE TABLE IF NOT EXISTS settings (
  key TEXT PRIMARY KEY,
  value_json TEXT NOT NULL CHECK(json_valid(value_json)),
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS chat_state (
  chat_id INTEGER PRIMARY KEY,
  list_revision INTEGER NOT NULL DEFAULT 0,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS conversation_messages (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  chat_id INTEGER NOT NULL,
  user_id INTEGER,
  role TEXT NOT NULL CHECK(role IN ('user','assistant','tool','system')),
  content TEXT NOT NULL,
  update_id INTEGER,
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS conversation_recent ON conversation_messages(chat_id,id DESC);
CREATE TABLE IF NOT EXISTS pending_actions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  chat_id INTEGER NOT NULL,
  user_id INTEGER NOT NULL,
  kind TEXT NOT NULL CHECK(kind IN ('clarify','confirm_add','confirm_clear')),
  payload_json TEXT NOT NULL CHECK(json_valid(payload_json)),
  list_revision INTEGER NOT NULL,
  expires_at TEXT NOT NULL,
  UNIQUE(chat_id,user_id)
);
CREATE TABLE IF NOT EXISTS telegram_updates (
  update_id INTEGER PRIMARY KEY,
  chat_id INTEGER NOT NULL,
  user_id INTEGER,
  payload_json TEXT NOT NULL CHECK(json_valid(payload_json)),
  state TEXT NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','processing','completed','failed')),
  attempts INTEGER NOT NULL DEFAULT 0,
  available_at TEXT,
  received_at TEXT NOT NULL,
  completed_at TEXT,
  error_code TEXT
);
CREATE INDEX IF NOT EXISTS updates_pending ON telegram_updates(state,available_at,update_id);
CREATE TABLE IF NOT EXISTS tool_actions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  update_id INTEGER NOT NULL REFERENCES telegram_updates(update_id),
  round_index INTEGER NOT NULL,
  action_index INTEGER NOT NULL,
  tool_name TEXT NOT NULL,
  arguments_json TEXT NOT NULL CHECK(json_valid(arguments_json)),
  result_json TEXT NOT NULL CHECK(json_valid(result_json)),
  created_at TEXT NOT NULL,
  UNIQUE(update_id,round_index,action_index)
);
CREATE TABLE IF NOT EXISTS llm_turns (
  update_id INTEGER NOT NULL REFERENCES telegram_updates(update_id),
  round_index INTEGER NOT NULL,
  request_json TEXT NOT NULL CHECK(json_valid(request_json)),
  response_json TEXT CHECK(response_json IS NULL OR json_valid(response_json)),
  state TEXT NOT NULL CHECK(state IN ('pending','completed','failed')),
  created_at TEXT NOT NULL,
  PRIMARY KEY(update_id,round_index)
);
CREATE TABLE IF NOT EXISTS outbox (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  update_id INTEGER NOT NULL REFERENCES telegram_updates(update_id),
  chat_id INTEGER NOT NULL,
  sequence INTEGER NOT NULL DEFAULT 0,
  payload_json TEXT NOT NULL CHECK(json_valid(payload_json)),
  state TEXT NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','sent','failed')),
  attempts INTEGER NOT NULL DEFAULT 0,
  telegram_message_id INTEGER,
  available_at TEXT,
  created_at TEXT NOT NULL,
  sent_at TEXT,
  UNIQUE(update_id,sequence)
);
CREATE INDEX IF NOT EXISTS outbox_pending ON outbox(state,available_at,id);
INSERT OR IGNORE INTO schema_migrations(version,applied_at) VALUES(1,strftime('%Y-%m-%dT%H:%M:%SZ','now'));
PRAGMA user_version=1;
