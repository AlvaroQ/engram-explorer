CREATE TABLE cloud_upgrade_state (
				project            TEXT PRIMARY KEY,
				stage              TEXT NOT NULL DEFAULT 'planned',
				repair_class       TEXT NOT NULL DEFAULT 'none',
				snapshot_json      TEXT NOT NULL DEFAULT '{}',
				last_error_code    TEXT,
				last_error_message TEXT,
				findings_json      TEXT,
				applied_actions    TEXT,
				updated_at         TEXT NOT NULL DEFAULT (datetime('now'))
			);
CREATE TABLE memory_relations (
			id                        INTEGER PRIMARY KEY AUTOINCREMENT,
			sync_id                   TEXT    NOT NULL UNIQUE,
			source_id                 TEXT,
			target_id                 TEXT,
			relation                  TEXT    NOT NULL DEFAULT 'pending',
			reason                    TEXT,
			evidence                  TEXT,
			confidence                REAL,
			judgment_status           TEXT    NOT NULL DEFAULT 'pending',
			marked_by_actor           TEXT,
			marked_by_kind            TEXT,
			marked_by_model           TEXT,
			session_id                TEXT,
			superseded_at             TEXT,
			superseded_by_relation_id INTEGER REFERENCES memory_relations(id) ON DELETE SET NULL,
			created_at                TEXT    NOT NULL DEFAULT (datetime('now')),
			updated_at                TEXT    NOT NULL DEFAULT (datetime('now'))
		);
CREATE TABLE observations (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT    NOT NULL,
			type       TEXT    NOT NULL,
			title      TEXT    NOT NULL,
			content    TEXT    NOT NULL,
			tool_name  TEXT,
			project    TEXT,
			scope      TEXT    NOT NULL DEFAULT 'project',
			topic_key  TEXT,
			normalized_hash TEXT,
			revision_count INTEGER NOT NULL DEFAULT 1,
			duplicate_count INTEGER NOT NULL DEFAULT 1,
			last_seen_at TEXT,
			created_at TEXT    NOT NULL DEFAULT (datetime('now')),
			updated_at TEXT    NOT NULL DEFAULT (datetime('now')),
			deleted_at TEXT, sync_id TEXT, review_after TEXT, expires_at TEXT, embedding BLOB, embedding_model TEXT, embedding_created_at TEXT,
			FOREIGN KEY (session_id) REFERENCES sessions(id)
		);
CREATE VIRTUAL TABLE observations_fts USING fts5(
			title,
			content,
			tool_name,
			type,
			project,
			topic_key,
			content='observations',
			content_rowid='id'
		);
CREATE TABLE 'observations_fts_config'(k PRIMARY KEY, v) WITHOUT ROWID;
CREATE TABLE 'observations_fts_data'(id INTEGER PRIMARY KEY, block BLOB);
CREATE TABLE 'observations_fts_docsize'(id INTEGER PRIMARY KEY, sz BLOB);
CREATE TABLE 'observations_fts_idx'(segid, term, pgno, PRIMARY KEY(segid, term)) WITHOUT ROWID;
CREATE TABLE prompt_tombstones (
				sync_id    TEXT PRIMARY KEY,
				session_id TEXT,
				project    TEXT,
				deleted_at TEXT NOT NULL DEFAULT (datetime('now'))
			);
CREATE VIRTUAL TABLE prompts_fts USING fts5(
			content,
			project,
			content='user_prompts',
			content_rowid='id'
		);
CREATE TABLE 'prompts_fts_config'(k PRIMARY KEY, v) WITHOUT ROWID;
CREATE TABLE 'prompts_fts_data'(id INTEGER PRIMARY KEY, block BLOB);
CREATE TABLE 'prompts_fts_docsize'(id INTEGER PRIMARY KEY, sz BLOB);
CREATE TABLE 'prompts_fts_idx'(segid, term, pgno, PRIMARY KEY(segid, term)) WITHOUT ROWID;
CREATE TABLE sessions (
			id         TEXT PRIMARY KEY,
			project    TEXT NOT NULL,
			directory  TEXT NOT NULL,
			started_at TEXT NOT NULL DEFAULT (datetime('now')),
			ended_at   TEXT,
			summary    TEXT
		);
CREATE TABLE sqlite_sequence(name,seq);
CREATE TABLE sync_apply_deferred (
			sync_id           TEXT    PRIMARY KEY,
			entity            TEXT    NOT NULL,
			payload           TEXT    NOT NULL,
			apply_status      TEXT    NOT NULL DEFAULT 'deferred',
			retry_count       INTEGER NOT NULL DEFAULT 0,
			last_error        TEXT,
			last_attempted_at TEXT,
			first_seen_at     TEXT    NOT NULL DEFAULT (datetime('now'))
		);
CREATE TABLE "sync_chunks" (
			target_key  TEXT NOT NULL DEFAULT 'local',
			chunk_id    TEXT NOT NULL,
			imported_at TEXT NOT NULL DEFAULT (datetime('now')),
			PRIMARY KEY (target_key, chunk_id)
		);
CREATE TABLE sync_enrolled_projects (
			project     TEXT PRIMARY KEY,
			enrolled_at TEXT NOT NULL DEFAULT (datetime('now'))
		);
CREATE TABLE sync_mutations (
				seq         INTEGER PRIMARY KEY AUTOINCREMENT,
				target_key  TEXT NOT NULL,
				entity      TEXT NOT NULL,
				entity_key  TEXT NOT NULL,
				op          TEXT NOT NULL,
				payload     TEXT NOT NULL,
				source      TEXT NOT NULL DEFAULT 'local',
				occurred_at TEXT NOT NULL DEFAULT (datetime('now')),
				acked_at    TEXT, project TEXT NOT NULL DEFAULT '',
				FOREIGN KEY (target_key) REFERENCES sync_state(target_key)
			);
CREATE TABLE sync_state (
				target_key           TEXT PRIMARY KEY,
				lifecycle            TEXT NOT NULL DEFAULT 'idle',
				last_enqueued_seq    INTEGER NOT NULL DEFAULT 0,
				last_acked_seq       INTEGER NOT NULL DEFAULT 0,
				last_pulled_seq      INTEGER NOT NULL DEFAULT 0,
				consecutive_failures INTEGER NOT NULL DEFAULT 0,
				backoff_until        TEXT,
				lease_owner          TEXT,
				lease_until          TEXT,
				last_error           TEXT,
				updated_at           TEXT NOT NULL DEFAULT (datetime('now'))
			, reason_code TEXT, reason_message TEXT);
CREATE TABLE user_prompts (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT    NOT NULL,
			content    TEXT    NOT NULL,
			project    TEXT,
			created_at TEXT    NOT NULL DEFAULT (datetime('now')), sync_id TEXT,
			FOREIGN KEY (session_id) REFERENCES sessions(id)
		);
CREATE INDEX idx_cloud_upgrade_state_stage ON cloud_upgrade_state(stage);
CREATE INDEX idx_memrel_source    ON memory_relations(source_id, judgment_status);
CREATE INDEX idx_memrel_status_created
			ON memory_relations(judgment_status, created_at DESC);
CREATE INDEX idx_memrel_supersede ON memory_relations(superseded_by_relation_id);
CREATE INDEX idx_memrel_target    ON memory_relations(target_id, judgment_status);
CREATE INDEX idx_obs_created  ON observations(created_at DESC);
CREATE INDEX idx_obs_dedupe ON observations(normalized_hash, project, scope, type, title, created_at DESC);
CREATE INDEX idx_obs_deleted ON observations(deleted_at);
CREATE INDEX idx_obs_project  ON observations(project);
CREATE INDEX idx_obs_scope ON observations(scope);
CREATE INDEX idx_obs_session  ON observations(session_id);
CREATE INDEX idx_obs_sync_id ON observations(sync_id);
CREATE INDEX idx_obs_topic ON observations(topic_key, project, scope, updated_at DESC);
CREATE INDEX idx_obs_type     ON observations(type);
CREATE INDEX idx_prompt_tombstones_project ON prompt_tombstones(project, deleted_at DESC);
CREATE INDEX idx_prompts_created ON user_prompts(created_at DESC);
CREATE INDEX idx_prompts_project ON user_prompts(project);
CREATE INDEX idx_prompts_session ON user_prompts(session_id);
CREATE INDEX idx_prompts_sync_id ON user_prompts(sync_id);
CREATE INDEX idx_sad_status_seen
			ON sync_apply_deferred(apply_status, first_seen_at);
CREATE INDEX idx_sync_mutations_lookup ON sync_mutations(target_key, entity, entity_key, source);
CREATE INDEX idx_sync_mutations_pending ON sync_mutations(target_key, acked_at, seq);
CREATE INDEX idx_sync_mutations_project ON sync_mutations(project);
CREATE INDEX idx_sync_mutations_target_seq ON sync_mutations(target_key, seq);
CREATE TRIGGER obs_fts_delete AFTER DELETE ON observations BEGIN
			INSERT INTO observations_fts(observations_fts, rowid, title, content, tool_name, type, project, topic_key)
			VALUES ('delete', old.id, old.title, old.content, old.tool_name, old.type, old.project, old.topic_key);
		END;
CREATE TRIGGER obs_fts_insert AFTER INSERT ON observations BEGIN
			INSERT INTO observations_fts(rowid, title, content, tool_name, type, project, topic_key)
			VALUES (new.id, new.title, new.content, new.tool_name, new.type, new.project, new.topic_key);
		END;
CREATE TRIGGER obs_fts_update AFTER UPDATE ON observations BEGIN
			INSERT INTO observations_fts(observations_fts, rowid, title, content, tool_name, type, project, topic_key)
			VALUES ('delete', old.id, old.title, old.content, old.tool_name, old.type, old.project, old.topic_key);
			INSERT INTO observations_fts(rowid, title, content, tool_name, type, project, topic_key)
			VALUES (new.id, new.title, new.content, new.tool_name, new.type, new.project, new.topic_key);
		END;
CREATE TRIGGER prompt_fts_delete AFTER DELETE ON user_prompts BEGIN
				INSERT INTO prompts_fts(prompts_fts, rowid, content, project)
				VALUES ('delete', old.id, old.content, old.project);
			END;
CREATE TRIGGER prompt_fts_insert AFTER INSERT ON user_prompts BEGIN
				INSERT INTO prompts_fts(rowid, content, project)
				VALUES (new.id, new.content, new.project);
			END;
CREATE TRIGGER prompt_fts_update AFTER UPDATE ON user_prompts BEGIN
				INSERT INTO prompts_fts(prompts_fts, rowid, content, project)
				VALUES ('delete', old.id, old.content, old.project);
				INSERT INTO prompts_fts(rowid, content, project)
				VALUES (new.id, new.content, new.project);
			END;
CREATE TRIGGER skip_empty_obs_upsert_mutations
BEFORE INSERT ON sync_mutations
FOR EACH ROW
WHEN NEW.entity = 'observation' AND NEW.op = 'upsert'
     AND (json_extract(NEW.payload, '$.content') IS NULL
          OR length(trim(coalesce(json_extract(NEW.payload, '$.content'), ''))) = 0)
BEGIN
  SELECT RAISE(IGNORE);
END;
