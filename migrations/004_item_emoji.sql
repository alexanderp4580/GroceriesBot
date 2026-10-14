-- Model-chosen emoji shown beside each item.
ALTER TABLE groceries ADD COLUMN emoji TEXT NOT NULL DEFAULT '';
