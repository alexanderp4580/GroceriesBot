-- Keep large clear/undo snapshots in SQLite rather than loading history into Go.
CREATE TABLE mutation_journal_items (
 journal_id INTEGER NOT NULL REFERENCES mutation_journal(id),
 item_id INTEGER NOT NULL REFERENCES groceries(id),
 before_json TEXT NOT NULL CHECK(json_valid(before_json)),
 after_version INTEGER NOT NULL,
 PRIMARY KEY(journal_id,item_id)
) STRICT;
