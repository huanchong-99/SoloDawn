ALTER TABLE planning_draft ADD COLUMN feishu_sync INTEGER NOT NULL DEFAULT 0;
ALTER TABLE planning_draft ADD COLUMN feishu_chat_id TEXT;
