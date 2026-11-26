package indexer

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"vex/internal/models"

	_ "modernc.org/sqlite"
)

// CollectionStats summarizes an indexed JSON collection.
type CollectionStats struct {
	Collection string   `json:"collection"`
	CardCount  int      `json:"cardCount"`
	Tags       []string `json:"tags"`
}

// SearchOptions configures a lexical search query against the SQLite FTS5 index.
type SearchOptions struct {
	Query      string                `json:"query"`
	Collection string                `json:"collection,omitempty"`
	Tags       []string              `json:"tags,omitempty"`
	Modifier   string                `json:"modifier,omitempty"`
	Layout     models.CardLayoutType `json:"layout,omitempty"`
	Limit      int                   `json:"limit,omitempty"`
}

// SearchResult pairs a matched SearchCard with ranking metadata.
type SearchResult struct {
	Card        models.SearchCard `json:"card"`
	Score       float64           `json:"score"`
	MatchSource string            `json:"matchSource"` // "exact", "contextual", "fts", "fuzzy", "filter"
}

// Store manages the embedded SQLite FTS5 index, entity_links graph, and card_recents table.
type Store struct {
	mu sync.RWMutex
	db *sql.DB
}

// OpenStore opens (or initializes) a SQLite index database. Pass ":memory:" for ephemeral tests.
func OpenStore(dbPath string) (*Store, error) {
	if dbPath == "" {
		return nil, errors.New("dbPath is required")
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// Single connection guarantees zero SQLITE_BUSY contention and preserves :memory: state.
	db.SetMaxOpenConns(1)

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the underlying SQLite connection.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

const currentSchemaVersion = 3

func normalizeEntityKey(raw string) (entityKey, entityCode string) {
	ek := strings.ToLower(strings.TrimSpace(raw))
	if ek == "" {
		return "", ""
	}
	if idx := strings.LastIndexByte(ek, ':'); idx >= 0 && idx+1 < len(ek) {
		return ek, strings.TrimSpace(ek[idx+1:])
	}
	return ek, ek
}

func marshalStringSliceJSON(items []string) string {
	if len(items) == 0 {
		return "[]"
	}
	raw, err := json.Marshal(items)
	if err != nil {
		return "[]"
	}
	return string(raw)
}

func (s *Store) migrate() error {
	// Connection-level PRAGMAs must execute outside an explicit transaction.
	pragmas := []string{
		`PRAGMA journal_mode = WAL;`,
		`PRAGMA busy_timeout = 5000;`,
		`PRAGMA foreign_keys = ON;`,
	}
	for _, stmt := range pragmas {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("configure sqlite (%s): %w", stmt, err)
		}
	}

	var userVersion int
	if err := s.db.QueryRow(`PRAGMA user_version;`).Scan(&userVersion); err != nil {
		return fmt.Errorf("read sqlite user_version: %w", err)
	}

	var cardsTableCount int
	if err := s.db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'cards';`).Scan(&cardsTableCount); err != nil {
		return fmt.Errorf("inspect sqlite tables: %w", err)
	}

	const createCardsTableSQL = `CREATE TABLE IF NOT EXISTS cards (
		id TEXT PRIMARY KEY,
		collection TEXT NOT NULL,
		layout TEXT NOT NULL,
		title TEXT NOT NULL,
		subtitle TEXT NOT NULL DEFAULT '',
		description TEXT NOT NULL DEFAULT '',
		entity_key TEXT NOT NULL DEFAULT '',
		entity_code TEXT NOT NULL DEFAULT '',
		parent_entity_key TEXT NOT NULL DEFAULT '',
		chips_text TEXT NOT NULL DEFAULT '',
		related_keys_json TEXT NOT NULL DEFAULT '[]',
		tags_json TEXT NOT NULL DEFAULT '[]',
		modifiers_json TEXT NOT NULL DEFAULT '[]',
		card_json TEXT NOT NULL,
		content_hash TEXT NOT NULL,
		source_file TEXT NOT NULL DEFAULT '',
		updated_at TEXT NOT NULL
	);`

	const createFTSTableSQL = `CREATE VIRTUAL TABLE IF NOT EXISTS cards_fts USING fts5(
		id UNINDEXED,
		collection UNINDEXED,
		entity_key,
		title,
		chips,
		modifiers,
		tags,
		subtitle,
		body,
		tokenize = 'porter unicode61'
	);`

	const createEntityLinksTableSQL = `CREATE TABLE IF NOT EXISTS entity_links (
		source_card_id TEXT NOT NULL,
		target_entity_key TEXT NOT NULL,
		relation_type TEXT NOT NULL,
		PRIMARY KEY (source_card_id, target_entity_key, relation_type),
		FOREIGN KEY (source_card_id) REFERENCES cards(id) ON DELETE CASCADE
	);`

	const createCardRecentsTableSQL = `CREATE TABLE IF NOT EXISTS card_recents (
		card_id TEXT PRIMARY KEY,
		last_used_at INTEGER NOT NULL
	);`

	indexStmts := []string{
		`CREATE INDEX IF NOT EXISTS idx_cards_collection ON cards(collection);`,
		`CREATE INDEX IF NOT EXISTS idx_cards_source_file ON cards(source_file);`,
		`CREATE INDEX IF NOT EXISTS idx_cards_entity_key ON cards(entity_key);`,
		`CREATE INDEX IF NOT EXISTS idx_cards_entity_code ON cards(entity_code);`,
		`CREATE INDEX IF NOT EXISTS idx_cards_parent_entity_key ON cards(parent_entity_key);`,
		`CREATE INDEX IF NOT EXISTS idx_cards_updated_at ON cards(updated_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_entity_links_target ON entity_links(target_entity_key, relation_type);`,
		`CREATE INDEX IF NOT EXISTS idx_card_recents_last_used ON card_recents(last_used_at DESC);`,
	}

	// Fast path 1: Already at current schema version.
	if cardsTableCount > 0 && userVersion >= currentSchemaVersion {
		ensureStmts := append([]string{createEntityLinksTableSQL, createCardRecentsTableSQL}, indexStmts...)
		for _, stmt := range ensureStmts {
			if _, err := s.db.Exec(stmt); err != nil {
				return fmt.Errorf("ensure sqlite schema (%s): %w", stmt[:min(40, len(stmt))], err)
			}
		}
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin sqlite migration: %w", err)
	}
	defer tx.Rollback()

	// Fast path 2: Brand-new database — initialize directly at schema v3.
	if cardsTableCount == 0 {
		initStmts := append(
			[]string{createCardsTableSQL, createFTSTableSQL, createEntityLinksTableSQL, createCardRecentsTableSQL},
			indexStmts...,
		)
		initStmts = append(initStmts, fmt.Sprintf(`PRAGMA user_version = %d;`, currentSchemaVersion))
		for _, stmt := range initStmts {
			if _, err := tx.Exec(stmt); err != nil {
				return fmt.Errorf("init sqlite v%d (%s): %w", currentSchemaVersion, stmt[:min(40, len(stmt))], err)
			}
		}
		return tx.Commit()
	}

	// Upgrade path: Legacy v0/v1/v2 database -> atomic in-place upgrade to v3.
	existingCols := make(map[string]bool)
	colRows, err := tx.Query(`PRAGMA table_info(cards);`)
	if err != nil {
		return fmt.Errorf("inspect cards columns: %w", err)
	}
	for colRows.Next() {
		var (
			cid     int
			name    string
			colType string
			notNull int
			dfltVal sql.NullString
			pk      int
		)
		if err := colRows.Scan(&cid, &name, &colType, &notNull, &dfltVal, &pk); err != nil {
			colRows.Close()
			return fmt.Errorf("scan cards table_info: %w", err)
		}
		existingCols[strings.ToLower(name)] = true
	}
	if err := colRows.Err(); err != nil {
		colRows.Close()
		return fmt.Errorf("iterate cards table_info: %w", err)
	}
	colRows.Close()

	// Rename legacy `template` column to `layout` when upgrading from v0/v1/v2.
	if existingCols["template"] && !existingCols["layout"] {
		if _, err := tx.Exec(`ALTER TABLE cards RENAME COLUMN template TO layout;`); err != nil {
			return fmt.Errorf("rename cards.template to layout: %w", err)
		}
		existingCols["layout"] = true
	}

	requiredCols := []struct {
		name string
		ddl  string
	}{
		{"layout", `ALTER TABLE cards ADD COLUMN layout TEXT NOT NULL DEFAULT 'facts';`},
		{"entity_key", `ALTER TABLE cards ADD COLUMN entity_key TEXT NOT NULL DEFAULT '';`},
		{"entity_code", `ALTER TABLE cards ADD COLUMN entity_code TEXT NOT NULL DEFAULT '';`},
		{"parent_entity_key", `ALTER TABLE cards ADD COLUMN parent_entity_key TEXT NOT NULL DEFAULT '';`},
		{"chips_text", `ALTER TABLE cards ADD COLUMN chips_text TEXT NOT NULL DEFAULT '';`},
		{"related_keys_json", `ALTER TABLE cards ADD COLUMN related_keys_json TEXT NOT NULL DEFAULT '[]';`},
	}
	for _, col := range requiredCols {
		if !existingCols[col.name] {
			if _, err := tx.Exec(col.ddl); err != nil {
				return fmt.Errorf("migrate cards add column %s: %w", col.name, err)
			}
		}
	}

	// Remove deprecated vector embeddings table.
	if _, err := tx.Exec(`DROP TABLE IF EXISTS card_embeddings;`); err != nil {
		return fmt.Errorf("drop deprecated card_embeddings: %w", err)
	}
	if _, err := tx.Exec(createEntityLinksTableSQL); err != nil {
		return fmt.Errorf("create entity_links: %w", err)
	}
	if _, err := tx.Exec(createCardRecentsTableSQL); err != nil {
		return fmt.Errorf("create card_recents: %w", err)
	}
	for _, stmt := range indexStmts {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("create sqlite index (%s): %w", stmt[:min(40, len(stmt))], err)
		}
	}

	// Rebuild cards_fts with the 9-column schema and refresh entity_links.
	if _, err := tx.Exec(`DROP TABLE IF EXISTS cards_fts;`); err != nil {
		return fmt.Errorf("drop legacy cards_fts: %w", err)
	}
	if _, err := tx.Exec(createFTSTableSQL); err != nil {
		return fmt.Errorf("create v3 cards_fts: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM entity_links;`); err != nil {
		return fmt.Errorf("clear entity_links for backfill: %w", err)
	}

	// Read all existing cards into memory before issuing writes on the single connection.
	type legacyRow struct {
		id            string
		collection    string
		title         string
		subtitle      string
		description   string
		tagsJSON      string
		modifiersJSON string
		cardJSON      string
	}
	rows, err := tx.Query(`SELECT id, collection, title, subtitle, description, tags_json, modifiers_json, card_json FROM cards`)
	if err != nil {
		return fmt.Errorf("read legacy cards for backfill: %w", err)
	}
	var existing []legacyRow
	for rows.Next() {
		var r legacyRow
		if err := rows.Scan(&r.id, &r.collection, &r.title, &r.subtitle, &r.description, &r.tagsJSON, &r.modifiersJSON, &r.cardJSON); err != nil {
			rows.Close()
			return fmt.Errorf("scan legacy card: %w", err)
		}
		existing = append(existing, r)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate legacy cards: %w", err)
	}
	rows.Close()

	for _, r := range existing {
		var card models.SearchCard
		if err := json.Unmarshal([]byte(r.cardJSON), &card); err == nil {
			entityKey, entityCode := normalizeEntityKey(card.EntityKey)
			parentKey, _ := normalizeEntityKey(card.ParentEntityKey)
			chipsText := card.ChipsText()
			relatedKeysJSON := marshalStringSliceJSON(card.RelatedKeys)
			if _, err := tx.Exec(`
				UPDATE cards
				SET entity_key = ?, entity_code = ?, parent_entity_key = ?, chips_text = ?, related_keys_json = ?
				WHERE id = ?
			`, entityKey, entityCode, parentKey, chipsText, relatedKeysJSON, r.id); err != nil {
				return fmt.Errorf("backfill card %q metadata: %w", r.id, err)
			}
			collection := card.Collection
			if collection == "" {
				collection = r.collection
			}
			if _, err := tx.Exec(`
				INSERT INTO cards_fts (id, collection, entity_key, title, chips, modifiers, tags, subtitle, body)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			`,
				r.id,
				collection,
				entityKey,
				card.Title,
				chipsText,
				strings.Join(card.Modifiers, " "),
				strings.Join(card.Tags, " "),
				card.Subtitle,
				card.SearchableBody(),
			); err != nil {
				return fmt.Errorf("backfill card %q fts: %w", r.id, err)
			}
			if err := syncEntityLinksTx(context.Background(), tx, r.id, parentKey, card.RelatedKeys); err != nil {
				return fmt.Errorf("backfill card %q entity_links: %w", r.id, err)
			}
		} else {
			var tags, mods []string
			_ = json.Unmarshal([]byte(r.tagsJSON), &tags)
			_ = json.Unmarshal([]byte(r.modifiersJSON), &mods)
			if _, err := tx.Exec(`
				INSERT INTO cards_fts (id, collection, entity_key, title, chips, modifiers, tags, subtitle, body)
				VALUES (?, ?, '', ?, '', ?, ?, ?, ?)
			`,
				r.id,
				r.collection,
				r.title,
				strings.Join(mods, " "),
				strings.Join(tags, " "),
				r.subtitle,
				r.description,
			); err != nil {
				return fmt.Errorf("backfill fallback card %q fts: %w", r.id, err)
			}
		}
	}

	if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d;`, currentSchemaVersion)); err != nil {
		return fmt.Errorf("set sqlite user_version=%d: %w", currentSchemaVersion, err)
	}
	return tx.Commit()
}

func syncEntityLinksTx(ctx context.Context, tx *sql.Tx, cardID, parentKey string, relatedKeys []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM entity_links WHERE source_card_id = ?`, cardID); err != nil {
		return err
	}
	if parentKey != "" {
		if _, err := tx.ExecContext(ctx, `
			INSERT OR IGNORE INTO entity_links (source_card_id, target_entity_key, relation_type)
			VALUES (?, ?, 'child_of')
		`, cardID, parentKey); err != nil {
			return err
		}
	}
	for _, rk := range relatedKeys {
		normRK, _ := normalizeEntityKey(rk)
		if normRK == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT OR IGNORE INTO entity_links (source_card_id, target_entity_key, relation_type)
			VALUES (?, ?, 'related_to')
		`, cardID, normRK); err != nil {
			return err
		}
	}
	return nil
}

// UpsertCard inserts or updates a single card in the main table, FTS5 index, and entity_links.
func (s *Store) UpsertCard(ctx context.Context, card models.SearchCard, sourceFile string) error {
	if err := models.ValidateAndNormalizeCard(&card, card.Collection); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := upsertCardTx(ctx, tx, card, sourceFile); err != nil {
		return err
	}
	return tx.Commit()
}

// ReplaceCollection synchronizes all cards belonging to a collection/sourceFile in a single transaction.
func (s *Store) ReplaceCollection(ctx context.Context, collection string, cards []models.SearchCard, sourceFile string) error {
	collection = models.Slugify(collection)

	normalized := make([]models.SearchCard, 0, len(cards))
	keepIDs := make(map[string]struct{}, len(cards))
	for _, c := range cards {
		if err := models.ValidateAndNormalizeCard(&c, collection); err != nil {
			return fmt.Errorf("card %q: %w", c.Title, err)
		}
		normalized = append(normalized, c)
		keepIDs[c.ID] = struct{}{}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var rows *sql.Rows
	if sourceFile != "" {
		rows, err = tx.QueryContext(ctx, `SELECT id FROM cards WHERE source_file = ?`, sourceFile)
	} else {
		rows, err = tx.QueryContext(ctx, `SELECT id FROM cards WHERE collection = ?`, collection)
	}
	if err != nil {
		return err
	}
	var existingIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		existingIDs = append(existingIDs, id)
	}
	rows.Close()

	for _, id := range existingIDs {
		if _, ok := keepIDs[id]; !ok {
			if _, err := tx.ExecContext(ctx, `DELETE FROM cards WHERE id = ?`, id); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM cards_fts WHERE id = ?`, id); err != nil {
				return err
			}
		}
	}

	for _, c := range normalized {
		if err := upsertCardTx(ctx, tx, c, sourceFile); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// ReplaceSourceFile atomically replaces all cards associated with a given sourceFile
// across any number of collections (used by the Composable Module & Overlay compiler).
func (s *Store) ReplaceSourceFile(ctx context.Context, sourceFile string, cards []models.SearchCard) error {
	normalized := make([]models.SearchCard, 0, len(cards))
	keepIDs := make(map[string]struct{}, len(cards))
	for _, c := range cards {
		if err := models.ValidateAndNormalizeCard(&c, c.Collection); err != nil {
			return err
		}
		normalized = append(normalized, c)
		keepIDs[c.ID] = struct{}{}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, `SELECT id FROM cards WHERE source_file = ?`, sourceFile)
	if err != nil {
		return err
	}
	var existingIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		existingIDs = append(existingIDs, id)
	}
	rows.Close()

	for _, id := range existingIDs {
		if _, ok := keepIDs[id]; !ok {
			if _, err := tx.ExecContext(ctx, `DELETE FROM cards WHERE id = ?`, id); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM cards_fts WHERE id = ?`, id); err != nil {
				return err
			}
		}
	}

	for _, c := range normalized {
		if err := upsertCardTx(ctx, tx, c, sourceFile); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// RecordCardUsed records or updates the last-used timestamp for cardID in card_recents.
func (s *Store) RecordCardUsed(ctx context.Context, cardID string) error {
	id := strings.TrimSpace(cardID)
	if id == "" {
		return nil
	}
	now := time.Now().UTC().UnixNano()

	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO card_recents (card_id, last_used_at)
		VALUES (?, ?)
		ON CONFLICT(card_id) DO UPDATE SET last_used_at = excluded.last_used_at
	`, id, now)
	return err
}

// ClearRecentCards removes all entries from card_recents.
func (s *Store) ClearRecentCards(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.ExecContext(ctx, `DELETE FROM card_recents`)
	return err
}

// ListRecentCards returns up to limit most recently used indexed cards (with up to 2 levels
// of flat-on-disk children dynamically assembled via entity_links).
func (s *Store) ListRecentCards(ctx context.Context, limit int) ([]models.SearchCard, error) {
	if limit <= 0 {
		limit = 6
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.QueryContext(ctx, `
		SELECT c.card_json
		FROM card_recents r
		JOIN cards c ON c.id = r.card_id
		ORDER BY r.last_used_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}

	var out []models.SearchCard
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			rows.Close()
			return nil, err
		}
		var card models.SearchCard
		if err := json.Unmarshal([]byte(raw), &card); err == nil {
			out = append(out, card)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	return s.hydrateChildrenLocked(ctx, out)
}

// FindRelatedCards returns up to limit other cards that share synergy with cardID
// via EntityKey, RelatedKeys, or distinctive domain tags.
func (s *Store) FindRelatedCards(ctx context.Context, cardID string, limit int) ([]models.SearchCard, error) {
	if limit <= 0 {
		limit = 5
	}
	target, err := s.GetCard(ctx, cardID)
	if err != nil || target == nil {
		return nil, err
	}

	needles := make(map[string]int) // token -> weight
	addToken := func(tok string, w int) {
		tok = strings.ToLower(strings.TrimSpace(tok))
		if tok == "" {
			return
		}
		if w > needles[tok] {
			needles[tok] = w
		}
		if idx := strings.IndexByte(tok, ':'); idx > 0 && idx+1 < len(tok) {
			short := tok[idx+1:]
			if w-1 > needles[short] {
				needles[short] = w - 1
			}
		}
	}

	addToken(target.EntityKey, 10)
	for _, rk := range target.RelatedKeys {
		addToken(rk, 8)
	}
	genericTags := map[string]bool{
		"port": true, "airport": true, "seaport": true, "iata": true, "unlocode": true,
		"europe": true, "asia": true, "usa": true, "container": true, "dimensions": true,
		"cbm": true, "hs": true, "hts": true, "customs": true, "tariff": true,
		"acronym": true, "ocean": true, "air": true, "portal": true, "dev": true, "web": true,
	}
	for _, t := range target.Tags {
		if !genericTags[t] {
			addToken(t, 5)
		}
	}
	if len(needles) == 0 {
		return nil, nil
	}

	type scoredCandidate struct {
		id    string
		title string
		score int
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, title, entity_key, related_keys_json, tags_json
		FROM cards
		WHERE id != ?
	`, target.ID)
	if err != nil {
		return nil, err
	}

	var matches []scoredCandidate
	for rows.Next() {
		var (
			id, title, entityKey, relatedRaw, tagsRaw string
		)
		if err := rows.Scan(&id, &title, &entityKey, &relatedRaw, &tagsRaw); err != nil {
			rows.Close()
			return nil, err
		}
		score := 0
		if entityKey != "" {
			if w, ok := needles[entityKey]; ok {
				score += w * 2
			}
		}
		if relatedRaw != "" && relatedRaw != "[]" && relatedRaw != "null" {
			var relKeys []string
			if err := json.Unmarshal([]byte(relatedRaw), &relKeys); err == nil {
				for _, rk := range relKeys {
					if w, ok := needles[strings.ToLower(strings.TrimSpace(rk))]; ok {
						score += w
					}
				}
			}
		}
		if tagsRaw != "" && tagsRaw != "[]" && tagsRaw != "null" {
			var tags []string
			if err := json.Unmarshal([]byte(tagsRaw), &tags); err == nil {
				for _, t := range tags {
					if w, ok := needles[strings.ToLower(strings.TrimSpace(t))]; ok {
						score += w
					}
				}
			}
		}
		if score >= 5 {
			matches = append(matches, scoredCandidate{id: id, title: title, score: score})
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	if len(matches) == 0 {
		return nil, nil
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		return matches[i].title < matches[j].title
	})

	if len(matches) > limit {
		matches = matches[:limit]
	}

	placeholders := make([]string, len(matches))
	args := make([]any, len(matches))
	for i, m := range matches {
		placeholders[i] = "?"
		args[i] = m.id
	}
	cardRows, err := s.db.QueryContext(ctx,
		`SELECT id, card_json FROM cards WHERE id IN (`+strings.Join(placeholders, ",")+`)`,
		args...,
	)
	if err != nil {
		return nil, err
	}

	byID := make(map[string]models.SearchCard, len(matches))
	for cardRows.Next() {
		var id, raw string
		if err := cardRows.Scan(&id, &raw); err != nil {
			cardRows.Close()
			return nil, err
		}
		var card models.SearchCard
		if err := json.Unmarshal([]byte(raw), &card); err == nil {
			byID[id] = card
		}
	}
	if err := cardRows.Err(); err != nil {
		cardRows.Close()
		return nil, err
	}
	cardRows.Close()

	out := make([]models.SearchCard, 0, len(matches))
	for _, m := range matches {
		if card, ok := byID[m.id]; ok {
			out = append(out, card)
		}
	}
	return s.hydrateChildrenLocked(ctx, out)
}

// DeleteBySourceFile removes all cards indexed from a deleted JSON file.
func (s *Store) DeleteBySourceFile(ctx context.Context, sourceFile string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM cards_fts WHERE id IN (SELECT id FROM cards WHERE source_file = ?)`, sourceFile); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM cards WHERE source_file = ?`, sourceFile); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteCard removes a single card by ID.
func (s *Store) DeleteCard(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM cards_fts WHERE id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM cards WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func upsertCardTx(ctx context.Context, tx *sql.Tx, card models.SearchCard, sourceFile string) error {
	tagsJSON := marshalStringSliceJSON(card.Tags)
	modsJSON := marshalStringSliceJSON(card.Modifiers)
	relatedKeysJSON := marshalStringSliceJSON(card.RelatedKeys)
	entityKey, entityCode := normalizeEntityKey(card.EntityKey)
	parentKey, _ := normalizeEntityKey(card.ParentEntityKey)
	chipsText := card.ChipsText()

	cardJSON, err := json.Marshal(card)
	if err != nil {
		return err
	}
	hash := card.ContentHash()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO cards (
			id, collection, layout, title, subtitle, description,
			entity_key, entity_code, parent_entity_key, chips_text, related_keys_json,
			tags_json, modifiers_json, card_json, content_hash, source_file, updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			collection = excluded.collection,
			layout = excluded.layout,
			title = excluded.title,
			subtitle = excluded.subtitle,
			description = excluded.description,
			entity_key = excluded.entity_key,
			entity_code = excluded.entity_code,
			parent_entity_key = excluded.parent_entity_key,
			chips_text = excluded.chips_text,
			related_keys_json = excluded.related_keys_json,
			tags_json = excluded.tags_json,
			modifiers_json = excluded.modifiers_json,
			card_json = excluded.card_json,
			content_hash = excluded.content_hash,
			source_file = excluded.source_file,
			updated_at = excluded.updated_at
	`,
		card.ID, card.Collection, string(card.Layout), card.Title, card.Subtitle, card.Description,
		entityKey, entityCode, parentKey, chipsText, relatedKeysJSON,
		tagsJSON, modsJSON, string(cardJSON), hash, sourceFile, card.UpdatedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return err
	}

	// Keep 9-column FTS5 row synchronized.
	if _, err := tx.ExecContext(ctx, `DELETE FROM cards_fts WHERE id = ?`, card.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO cards_fts (id, collection, entity_key, title, chips, modifiers, tags, subtitle, body)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		card.ID,
		card.Collection,
		entityKey,
		card.Title,
		chipsText,
		strings.Join(card.Modifiers, " "),
		strings.Join(card.Tags, " "),
		card.Subtitle,
		card.SearchableBody(),
	); err != nil {
		return err
	}

	return syncEntityLinksTx(ctx, tx, card.ID, parentKey, card.RelatedKeys)
}

// GetCard retrieves a card by its ID with up to 2 levels of dynamic entity_links children hydrated.
func (s *Store) GetCard(ctx context.Context, id string) (*models.SearchCard, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT card_json FROM cards WHERE id = ?`, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var card models.SearchCard
	if err := json.Unmarshal([]byte(raw), &card); err != nil {
		return nil, err
	}
	hydrated, err := s.hydrateChildrenLocked(ctx, []models.SearchCard{card})
	if err != nil {
		return nil, err
	}
	return &hydrated[0], nil
}

// ListCards returns all cards (optionally filtered by collection).
func (s *Store) ListCards(ctx context.Context, collection string) ([]models.SearchCard, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var rows *sql.Rows
	var err error
	if collection != "" {
		rows, err = s.db.QueryContext(ctx, `SELECT card_json FROM cards WHERE collection = ? ORDER BY title ASC`, models.Slugify(collection))
	} else {
		rows, err = s.db.QueryContext(ctx, `SELECT card_json FROM cards ORDER BY collection ASC, title ASC`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.SearchCard
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var card models.SearchCard
		if err := json.Unmarshal([]byte(raw), &card); err == nil {
			out = append(out, card)
		}
	}
	return out, rows.Err()
}

// ListCollections returns aggregate stats per collection.
func (s *Store) ListCollections(ctx context.Context) ([]CollectionStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.QueryContext(ctx, `SELECT collection, tags_json FROM cards ORDER BY collection ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	statsMap := make(map[string]*CollectionStats)
	tagSets := make(map[string]map[string]struct{})
	var order []string

	for rows.Next() {
		var col, tagsRaw string
		if err := rows.Scan(&col, &tagsRaw); err != nil {
			return nil, err
		}
		st, ok := statsMap[col]
		if !ok {
			st = &CollectionStats{Collection: col}
			statsMap[col] = st
			tagSets[col] = make(map[string]struct{})
			order = append(order, col)
		}
		st.CardCount++
		var tags []string
		if err := json.Unmarshal([]byte(tagsRaw), &tags); err == nil {
			for _, t := range tags {
				tagSets[col][t] = struct{}{}
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]CollectionStats, 0, len(order))
	for _, col := range order {
		st := statsMap[col]
		for t := range tagSets[col] {
			st.Tags = append(st.Tags, t)
		}
		sort.Strings(st.Tags)
		out = append(out, *st)
	}
	return out, nil
}

// hydrateChildrenLocked dynamically assembles up to 2 levels of child cards (L0 -> L1 -> L2)
// from entity_links (relation_type = 'child_of') in at most 2 batched SQL queries.
func (s *Store) hydrateChildrenLocked(ctx context.Context, cards []models.SearchCard) ([]models.SearchCard, error) {
	if len(cards) == 0 {
		return cards, nil
	}

	// Level 1: collect normalized entityKeys from root cards.
	rootKeys := collectEntityKeys(cards)
	l1ByParent, err := s.queryChildrenByEntityKeysLocked(ctx, rootKeys)
	if err != nil {
		return nil, err
	}

	// Attach Level 1 children to each root card (preserving any inline children and deduplicating by ID).
	var allL1 []models.SearchCard
	for i := range cards {
		seen := map[string]bool{cards[i].ID: true}
		for _, inline := range cards[i].Children {
			seen[inline.ID] = true
		}
		if ek, _ := normalizeEntityKey(cards[i].EntityKey); ek != "" {
			for _, child := range l1ByParent[ek] {
				if seen[child.ID] {
					continue
				}
				seen[child.ID] = true
				child.Children = append([]models.SearchCard(nil), child.Children...)
				cards[i].Children = append(cards[i].Children, child)
			}
		}
		allL1 = append(allL1, cards[i].Children...)
	}

	if len(allL1) == 0 {
		return cards, nil
	}

	// Level 2: collect normalized entityKeys from all Level 1 children.
	l1Keys := collectEntityKeys(allL1)
	l2ByParent, err := s.queryChildrenByEntityKeysLocked(ctx, l1Keys)
	if err != nil {
		return nil, err
	}

	// Attach Level 2 grandchildren and enforce depth cap <= 2.
	for i := range cards {
		rootID := cards[i].ID
		for j := range cards[i].Children {
			l1 := &cards[i].Children[j]
			seen := map[string]bool{rootID: true, l1.ID: true}
			// Trim any inline depth > 2 children.
			for k := range l1.Children {
				seen[l1.Children[k].ID] = true
				l1.Children[k].Children = nil
			}
			if ek, _ := normalizeEntityKey(l1.EntityKey); ek != "" {
				for _, grand := range l2ByParent[ek] {
					if seen[grand.ID] {
						continue
					}
					seen[grand.ID] = true
					grand.Children = nil // Hard depth cap at L2.
					l1.Children = append(l1.Children, grand)
				}
			}
		}
	}

	return cards, nil
}

func collectEntityKeys(cards []models.SearchCard) []string {
	seen := make(map[string]struct{}, len(cards))
	var out []string
	for _, c := range cards {
		if ek, _ := normalizeEntityKey(c.EntityKey); ek != "" {
			if _, ok := seen[ek]; !ok {
				seen[ek] = struct{}{}
				out = append(out, ek)
			}
		}
	}
	return out
}

func (s *Store) queryChildrenByEntityKeysLocked(ctx context.Context, parentKeys []string) (map[string][]models.SearchCard, error) {
	if len(parentKeys) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(parentKeys))
	args := make([]any, len(parentKeys))
	for i, k := range parentKeys {
		placeholders[i] = "?"
		args[i] = k
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT el.target_entity_key, c.card_json
		FROM entity_links el
		JOIN cards c ON c.id = el.source_card_id
		WHERE el.relation_type = 'child_of'
		  AND el.target_entity_key IN (`+strings.Join(placeholders, ",")+`)
		ORDER BY c.title ASC, c.id ASC
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	grouped := make(map[string][]models.SearchCard, len(parentKeys))
	for rows.Next() {
		var parentKey, raw string
		if err := rows.Scan(&parentKey, &raw); err != nil {
			return nil, err
		}
		var child models.SearchCard
		if err := json.Unmarshal([]byte(raw), &child); err == nil {
			grouped[parentKey] = append(grouped[parentKey], child)
		}
	}
	return grouped, rows.Err()
}

// SearchHybrid executes a 4-tier exact/contextual/prefix/guarded-fuzzy lexical search
// with dynamic relevance cliff and flat-on-disk 2-level child hydration via entity_links.
func (s *Store) SearchHybrid(ctx context.Context, opts SearchOptions) ([]SearchResult, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}
	query := strings.TrimSpace(opts.Query)

	s.mu.RLock()
	defer s.mu.RUnlock()

	// If no text query, return filtered root cards sorted by updated_at desc.
	if query == "" {
		return s.searchFilterOnlyLocked(ctx, opts, limit)
	}

	type rankEntry struct {
		card          models.SearchCard
		lexicalScore  float64
		lexicalTier   int    // 1=exact, 2=fts-exact/contextual, 3=fts-prefix, 4=fuzzy
		lexicalSource string // "exact", "contextual", "fts", "fuzzy"
	}

	merged := make(map[string]*rankEntry)
	recordLexical := func(card models.SearchCard, score float64, tier int, source string) {
		entry, ok := merged[card.ID]
		if !ok {
			entry = &rankEntry{card: card}
			merged[card.ID] = entry
		}
		if score > entry.lexicalScore {
			entry.lexicalScore = score
			entry.lexicalTier = tier
			entry.lexicalSource = source
		}
	}

	candidateLimit := limit * 3
	if candidateLimit < 30 {
		candidateLimit = 30
	}

	tokens := tokenizeQuery(query)
	exactFTSQuery := buildFTS5QueryFromTokens(tokens, false)
	prefixFTSQuery := buildFTS5QueryFromTokens(tokens, true)

	// Tier 1 (88.0 - 100.0): Exact ID, entity_key, entity_code, title, modifier, tag, chip, or (CODE) match.
	exactHits, exactSeeds, err := s.searchExactLocked(ctx, query, exactFTSQuery, opts, candidateLimit)
	if err == nil {
		for _, hit := range exactHits {
			recordLexical(hit.card, hit.score, 1, "exact")
		}
	}

	// Tier 2a (45.0 - 75.0): Whole-token FTS5 BM25 match (no trailing prefix wildcard).
	if exactFTSQuery != "" {
		ftsExactHits, err := s.searchFTSLocked(ctx, exactFTSQuery, opts, candidateLimit)
		if err == nil && len(ftsExactHits) > 0 {
			bestMag := math.Abs(ftsExactHits[0].bm25)
			if bestMag < 1e-6 {
				bestMag = 1.0
			}
			for rank, hit := range ftsExactHits {
				relQuality := math.Min(1.0, math.Abs(hit.bm25)/bestMag)
				rankDecay := 1.0 / (1.0 + 0.12*float64(rank))
				score := 45.0 + 30.0*(0.65*relQuality+0.35*rankDecay)
				recordLexical(hit.card, score, 2, "fts")
			}
		}
	}

	// Tier 2b (52.0 - 78.0): 1-hop contextual neighbors linked to Tier 1 exact entity hits.
	if len(exactSeeds) > 0 {
		neighbors, err := s.searchContextualNeighborsLocked(ctx, exactSeeds, opts, 4)
		if err == nil {
			for rank, card := range neighbors {
				base := 58.0 - float64(rank)*1.5
				if existing, ok := merged[card.ID]; ok {
					if existing.lexicalTier == 1 {
						continue
					}
					if existing.lexicalTier == 2 {
						existing.lexicalScore = math.Min(78.0, math.Max(existing.lexicalScore+3.0, base))
						existing.lexicalSource = "contextual"
						continue
					}
				}
				recordLexical(card, base, 2, "contextual")
			}
		}
	}

	// Tier 3 (18.0 - 38.0 header prefix; 16.0 - 24.0 body-only prefix): Prefix FTS5 match.
	if prefixFTSQuery != "" {
		ftsPrefixHits, err := s.searchFTSLocked(ctx, prefixFTSQuery, opts, candidateLimit)
		if err == nil && len(ftsPrefixHits) > 0 {
			bestMag := math.Abs(ftsPrefixHits[0].bm25)
			if bestMag < 1e-6 {
				bestMag = 1.0
			}
			for rank, hit := range ftsPrefixHits {
				relQuality := math.Min(1.0, math.Abs(hit.bm25)/bestMag)
				rankDecay := 1.0 / (1.0 + 0.12*float64(rank))
				combined := 0.65*relQuality + 0.35*rankDecay
				var score float64
				if hasHeaderPrefixMatch(hit.card, tokens) {
					score = 18.0 + 20.0*combined
				} else {
					score = 16.0 + 8.0*combined
				}
				recordLexical(hit.card, score, 3, "fts")
			}
		}
	}

	// Tier 4 (5.0 - 14.0): Guarded metadata-only substring fallback (NEVER scans card_json).
	if len(merged) < 3 && len([]rune(query)) >= 3 {
		fuzzyCards, err := s.searchFuzzyLocked(ctx, query, opts, candidateLimit)
		if err == nil {
			for rank, card := range fuzzyCards {
				score := 14.0 - math.Min(9.0, float64(rank)*1.2)
				recordLexical(card, score, 4, "fuzzy")
			}
		}
	}

	results := make([]SearchResult, 0, len(merged))
	for _, e := range merged {
		source := e.lexicalSource
		if source == "" {
			source = "fts"
		}
		results = append(results, SearchResult{
			Card:        e.card,
			Score:       e.lexicalScore,
			MatchSource: source,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return results[i].Card.Title < results[j].Card.Title
	})

	// Dynamic relevance cliff: prune low-signal tail sprawl on interactive lexical queries.
	if len(results) > 1 {
		topScore := results[0].Score
		var cutoff float64
		switch {
		case topScore >= 80.0:
			cutoff = 0.25 * topScore
		case topScore >= 45.0:
			cutoff = 0.30 * topScore
		}
		if cutoff > 0 {
			pruned := results[:0]
			for _, r := range results {
				if r.Score >= cutoff {
					pruned = append(pruned, r)
				}
			}
			results = pruned
		}
	}

	// Hydrate up to 2 levels of children via entity_links.
	rawCards := make([]models.SearchCard, len(results))
	for i := range results {
		rawCards[i] = results[i].Card
	}
	hydrated, err := s.hydrateChildrenLocked(ctx, rawCards)
	if err != nil {
		return nil, err
	}
	for i := range results {
		results[i].Card = hydrated[i]
	}

	// Deduplicate flat child cards that are already nested under a parent card present in results.
	nestedChildIDs := make(map[string]bool)
	for _, r := range results {
		for _, ch := range r.Card.Children {
			nestedChildIDs[ch.ID] = true
			for _, grand := range ch.Children {
				nestedChildIDs[grand.ID] = true
			}
		}
	}
	if len(nestedChildIDs) > 0 {
		deduped := results[:0]
		for _, r := range results {
			if r.Card.ParentEntityKey != "" && nestedChildIDs[r.Card.ID] {
				continue
			}
			deduped = append(deduped, r)
		}
		results = deduped
	}

	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

type exactCandidate struct {
	card  models.SearchCard
	score float64
}

func (s *Store) searchExactLocked(ctx context.Context, query, exactFTSQuery string, opts SearchOptions, limit int) ([]exactCandidate, []models.SearchCard, error) {
	qLower := strings.ToLower(strings.TrimSpace(query))
	if qLower == "" {
		return nil, nil, nil
	}

	seen := make(map[string]exactCandidate)
	seedsByID := make(map[string]models.SearchCard)

	consider := func(raw, entityKey, entityCode string) {
		var card models.SearchCard
		if err := json.Unmarshal([]byte(raw), &card); err != nil {
			return
		}
		if !matchesFilters(card, opts) {
			return
		}
		score, isSeed := scoreExactCard(card, entityKey, entityCode, qLower)
		if score <= 0 {
			return
		}
		if prev, ok := seen[card.ID]; !ok || score > prev.score {
			seen[card.ID] = exactCandidate{card: card, score: score}
		}
		if isSeed {
			seedsByID[card.ID] = card
		}
	}

	// Pass 1: Fast B-tree indexed lookup on id, entity_key, entity_code, and full title.
	rows, err := s.db.QueryContext(ctx, `
		SELECT card_json, entity_key, entity_code
		FROM cards
		WHERE lower(id) = ?
		   OR entity_key = ?
		   OR entity_code = ?
		   OR lower(title) = ?
	`, qLower, qLower, qLower, qLower)
	if err != nil {
		return nil, nil, err
	}
	for rows.Next() {
		var raw, ek, ec string
		if err := rows.Scan(&raw, &ek, &ec); err != nil {
			rows.Close()
			return nil, nil, err
		}
		consider(raw, ek, ec)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, nil, err
	}
	rows.Close()

	// Pass 2: Column-scoped FTS5 exact-token lookup across structured metadata columns.
	if exactFTSQuery != "" {
		scopedQuery := `{entity_key title chips modifiers tags} : (` + exactFTSQuery + `)`
		ftsRows, err := s.db.QueryContext(ctx, `
			SELECT c.card_json, c.entity_key, c.entity_code
			FROM cards_fts f
			JOIN cards c ON c.id = f.id
			WHERE cards_fts MATCH ?
			ORDER BY bm25(cards_fts, 0.0, 0.0, 14.0, 12.0, 10.0, 9.0, 9.0, 6.0, 2.0) ASC
			LIMIT ?
		`, scopedQuery, limit*2)
		if err == nil {
			for ftsRows.Next() {
				var raw, ek, ec string
				if err := ftsRows.Scan(&raw, &ek, &ec); err != nil {
					ftsRows.Close()
					return nil, nil, err
				}
				consider(raw, ek, ec)
			}
			_ = ftsRows.Err()
			ftsRows.Close()
		}
	}

	out := make([]exactCandidate, 0, len(seen))
	for _, c := range seen {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].score != out[j].score {
			return out[i].score > out[j].score
		}
		return out[i].card.Title < out[j].card.Title
	})
	if len(out) > limit {
		out = out[:limit]
	}

	seeds := make([]models.SearchCard, 0, len(seedsByID))
	for _, c := range seedsByID {
		seeds = append(seeds, c)
	}
	return out, seeds, nil
}

func scoreExactCard(card models.SearchCard, entityKey, entityCode, qLower string) (score float64, isSeed bool) {
	hasGraph := strings.TrimSpace(card.EntityKey) != "" || len(card.RelatedKeys) > 0

	if strings.ToLower(strings.TrimSpace(card.ID)) == qLower || (entityKey != "" && entityKey == qLower) {
		return 100.0, hasGraph
	}
	if entityCode != "" && entityCode == qLower {
		return 98.0, hasGraph
	}
	if strings.ToLower(strings.TrimSpace(card.Title)) == qLower {
		return 96.0, hasGraph
	}
	for _, m := range card.Modifiers {
		if strings.ToLower(strings.TrimSpace(m)) == qLower {
			return 92.0, false
		}
	}
	for _, t := range card.Tags {
		if strings.ToLower(strings.TrimSpace(t)) == qLower {
			return 92.0, false
		}
	}
	if hasExactChipSegment(card.Chips, qLower) {
		return 90.0, hasGraph
	}
	if hasExactParenthesizedCode(card.Title, qLower) {
		return 88.0, hasGraph
	}
	return 0, false
}

func hasExactChipSegment(chips []models.GlanceChip, qLower string) bool {
	for _, ch := range chips {
		lbl := strings.ToLower(strings.TrimSpace(ch.Label))
		if lbl == "" {
			continue
		}
		if lbl == qLower {
			return true
		}
		parts := strings.FieldsFunc(lbl, func(r rune) bool {
			return r == '·' || r == '|' || r == '/' || r == ','
		})
		for _, p := range parts {
			if strings.TrimSpace(p) == qLower {
				return true
			}
		}
	}
	return false
}

func hasExactParenthesizedCode(title, qLower string) bool {
	lowerTitle := strings.ToLower(title)
	return strings.Contains(lowerTitle, "("+qLower+")") || strings.Contains(lowerTitle, "["+qLower+"]")
}

func hasHeaderPrefixMatch(card models.SearchCard, queryTokens []string) bool {
	if len(queryTokens) == 0 {
		return false
	}
	var b strings.Builder
	b.WriteString(card.Title)
	b.WriteByte(' ')
	b.WriteString(card.Subtitle)
	b.WriteByte(' ')
	b.WriteString(card.EntityKey)
	b.WriteByte(' ')
	b.WriteString(card.ChipsText())
	for _, t := range card.Tags {
		b.WriteByte(' ')
		b.WriteString(t)
	}
	for _, m := range card.Modifiers {
		b.WriteByte(' ')
		b.WriteString(m)
	}
	headerTokens := tokenizeQuery(b.String())
	if len(headerTokens) == 0 {
		return false
	}
	for _, qt := range queryTokens {
		matched := false
		for _, ht := range headerTokens {
			if strings.HasPrefix(ht, qt) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func (s *Store) searchContextualNeighborsLocked(ctx context.Context, seeds []models.SearchCard, opts SearchOptions, maxNeighbors int) ([]models.SearchCard, error) {
	if len(seeds) == 0 || maxNeighbors <= 0 {
		return nil, nil
	}
	seedIDs := make(map[string]struct{}, len(seeds))
	seedEntityKeys := make(map[string]struct{}, len(seeds))
	seedRelatedKeys := make(map[string]struct{})

	for _, seed := range seeds {
		seedIDs[seed.ID] = struct{}{}
		if ek, _ := normalizeEntityKey(seed.EntityKey); ek != "" {
			seedEntityKeys[ek] = struct{}{}
		}
		for _, rk := range seed.RelatedKeys {
			if norm, _ := normalizeEntityKey(rk); norm != "" {
				seedRelatedKeys[norm] = struct{}{}
			}
		}
	}
	if len(seedEntityKeys) == 0 && len(seedRelatedKeys) == 0 {
		return nil, nil
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, title, entity_key, parent_entity_key, related_keys_json
		FROM cards
	`)
	if err != nil {
		return nil, err
	}

	type neighborCandidate struct {
		id       string
		title    string
		strength int
	}
	var candidates []neighborCandidate
	for rows.Next() {
		var id, title, entityKey, parentKey, relatedRaw string
		if err := rows.Scan(&id, &title, &entityKey, &parentKey, &relatedRaw); err != nil {
			rows.Close()
			return nil, err
		}
		if _, isSeed := seedIDs[id]; isSeed {
			continue
		}
		// Cards directly attached as child_of a seed will be hydrated inside seed.Children instead of duplicated as siblings.
		if parentKey != "" {
			if _, isDirectChild := seedEntityKeys[parentKey]; isDirectChild {
				continue
			}
		}
		strength := 0
		if entityKey != "" {
			if _, ok := seedRelatedKeys[entityKey]; ok {
				strength += 2
			}
		}
		if relatedRaw != "" && relatedRaw != "[]" && relatedRaw != "null" {
			var relKeys []string
			if err := json.Unmarshal([]byte(relatedRaw), &relKeys); err == nil {
				for _, rk := range relKeys {
					norm, _ := normalizeEntityKey(rk)
					if norm == "" {
						continue
					}
					if _, ok := seedEntityKeys[norm]; ok {
						strength += 2
					} else if _, ok := seedRelatedKeys[norm]; ok {
						strength += 1
					}
				}
			}
		}
		if strength > 0 {
			candidates = append(candidates, neighborCandidate{
				id:       id,
				title:    title,
				strength: strength,
			})
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	if len(candidates) == 0 {
		return nil, nil
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].strength != candidates[j].strength {
			return candidates[i].strength > candidates[j].strength
		}
		return candidates[i].title < candidates[j].title
	})

	fetchCap := maxNeighbors * 4
	if len(candidates) > fetchCap {
		candidates = candidates[:fetchCap]
	}

	placeholders := make([]string, len(candidates))
	args := make([]any, len(candidates))
	for i, c := range candidates {
		placeholders[i] = "?"
		args[i] = c.id
	}
	cardRows, err := s.db.QueryContext(ctx,
		`SELECT id, card_json FROM cards WHERE id IN (`+strings.Join(placeholders, ",")+`)`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer cardRows.Close()

	byID := make(map[string]models.SearchCard, len(candidates))
	for cardRows.Next() {
		var id, raw string
		if err := cardRows.Scan(&id, &raw); err != nil {
			return nil, err
		}
		var card models.SearchCard
		if err := json.Unmarshal([]byte(raw), &card); err == nil {
			byID[id] = card
		}
	}
	if err := cardRows.Err(); err != nil {
		return nil, err
	}

	out := make([]models.SearchCard, 0, maxNeighbors)
	for _, c := range candidates {
		card, ok := byID[c.id]
		if !ok || !matchesFilters(card, opts) {
			continue
		}
		out = append(out, card)
		if len(out) >= maxNeighbors {
			break
		}
	}
	return out, nil
}

func (s *Store) searchFilterOnlyLocked(ctx context.Context, opts SearchOptions, limit int) ([]SearchResult, error) {
	// When browsing without an explicit layout/tag/modifier filter, hide flat child cards whose parent
	// exists in the index so they render cleanly nested under their parent instead of cluttering top-level browse.
	hasExplicitFacet := opts.Layout != "" || opts.Modifier != "" || len(opts.Tags) > 0
	var querySQL string
	if hasExplicitFacet {
		querySQL = `SELECT card_json FROM cards ORDER BY updated_at DESC, title ASC`
	} else {
		querySQL = `
			SELECT c.card_json
			FROM cards c
			WHERE c.parent_entity_key = ''
			   OR NOT EXISTS (SELECT 1 FROM cards p WHERE p.entity_key = c.parent_entity_key)
			ORDER BY c.updated_at DESC, c.title ASC
		`
	}

	rows, err := s.db.QueryContext(ctx, querySQL)
	if err != nil {
		return nil, err
	}

	var matchedCards []models.SearchCard
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			rows.Close()
			return nil, err
		}
		var card models.SearchCard
		if err := json.Unmarshal([]byte(raw), &card); err != nil {
			continue
		}
		if !matchesFilters(card, opts) {
			continue
		}
		matchedCards = append(matchedCards, card)
		if len(matchedCards) >= limit {
			break
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	hydrated, err := s.hydrateChildrenLocked(ctx, matchedCards)
	if err != nil {
		return nil, err
	}
	out := make([]SearchResult, 0, len(hydrated))
	for _, card := range hydrated {
		out = append(out, SearchResult{
			Card:        card,
			Score:       1.0,
			MatchSource: "filter",
		})
	}
	return out, nil
}

type ftsCandidate struct {
	card models.SearchCard
	bm25 float64
}

func (s *Store) searchFTSLocked(ctx context.Context, ftsQuery string, opts SearchOptions, limit int) ([]ftsCandidate, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.card_json, bm25(cards_fts, 0.0, 0.0, 14.0, 12.0, 10.0, 9.0, 9.0, 6.0, 2.0) AS rank_score
		FROM cards_fts f
		JOIN cards c ON c.id = f.id
		WHERE cards_fts MATCH ?
		ORDER BY rank_score ASC
		LIMIT ?
	`, ftsQuery, limit*2)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ftsCandidate
	for rows.Next() {
		var raw string
		var bm25Score float64
		if err := rows.Scan(&raw, &bm25Score); err != nil {
			return nil, err
		}
		var card models.SearchCard
		if err := json.Unmarshal([]byte(raw), &card); err != nil {
			continue
		}
		if !matchesFilters(card, opts) {
			continue
		}
		out = append(out, ftsCandidate{card: card, bm25: bm25Score})
		if len(out) >= limit {
			break
		}
	}
	return out, rows.Err()
}

func escapeLikePattern(raw string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + r.Replace(strings.ToLower(strings.TrimSpace(raw))) + "%"
}

func (s *Store) searchFuzzyLocked(ctx context.Context, query string, opts SearchOptions, limit int) ([]models.SearchCard, error) {
	pattern := escapeLikePattern(query)
	rows, err := s.db.QueryContext(ctx, `
		SELECT card_json
		FROM cards
		WHERE lower(title) LIKE ? ESCAPE '\'
		   OR lower(subtitle) LIKE ? ESCAPE '\'
		   OR entity_key LIKE ? ESCAPE '\'
		   OR lower(chips_text) LIKE ? ESCAPE '\'
		   OR lower(tags_json) LIKE ? ESCAPE '\'
		ORDER BY
			CASE
				WHEN lower(title) LIKE ? ESCAPE '\' THEN 0
				WHEN entity_key LIKE ? ESCAPE '\' OR lower(chips_text) LIKE ? ESCAPE '\' THEN 1
				ELSE 2
			END,
			title ASC
		LIMIT ?
	`, pattern, pattern, pattern, pattern, pattern, pattern, pattern, pattern, limit*2)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.SearchCard
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var card models.SearchCard
		if err := json.Unmarshal([]byte(raw), &card); err != nil {
			continue
		}
		if !matchesFilters(card, opts) {
			continue
		}
		out = append(out, card)
		if len(out) >= limit {
			break
		}
	}
	return out, rows.Err()
}

// FindCardsByEntityKeys returns indexed SearchCards whose EntityKey (or EntityCode when given a bare code)
// matches any of the requested keys (case-insensitive), with up to 2 levels of entity_links children hydrated.
func (s *Store) FindCardsByEntityKeys(ctx context.Context, entityKeys []string) ([]models.SearchCard, error) {
	seenFull := make(map[string]struct{}, len(entityKeys))
	seenBare := make(map[string]struct{}, len(entityKeys))
	fullKeys := make([]string, 0, len(entityKeys))
	bareCodes := make([]string, 0, len(entityKeys))

	for _, k := range entityKeys {
		clean := strings.ToLower(strings.TrimSpace(k))
		if clean == "" {
			continue
		}
		if _, ok := seenFull[clean]; !ok {
			seenFull[clean] = struct{}{}
			fullKeys = append(fullKeys, clean)
		}
		if !strings.ContainsRune(clean, ':') {
			if _, ok := seenBare[clean]; !ok {
				seenBare[clean] = struct{}{}
				bareCodes = append(bareCodes, clean)
			}
		}
	}
	if len(fullKeys) == 0 {
		return nil, nil
	}

	fullPlaceholders := make([]string, len(fullKeys))
	args := make([]any, 0, len(fullKeys)+len(bareCodes))
	for i, k := range fullKeys {
		fullPlaceholders[i] = "?"
		args = append(args, k)
	}

	query := `SELECT card_json FROM cards WHERE entity_key IN (` + strings.Join(fullPlaceholders, ",") + `)`
	if len(bareCodes) > 0 {
		barePlaceholders := make([]string, len(bareCodes))
		for i, c := range bareCodes {
			barePlaceholders[i] = "?"
			args = append(args, c)
		}
		query += ` OR entity_code IN (` + strings.Join(barePlaceholders, ",") + `)`
	}
	query += ` ORDER BY title ASC`

	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}

	var out []models.SearchCard
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			rows.Close()
			return nil, err
		}
		var card models.SearchCard
		if err := json.Unmarshal([]byte(raw), &card); err == nil {
			out = append(out, card)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	return s.hydrateChildrenLocked(ctx, out)
}

func matchesFilters(card models.SearchCard, opts SearchOptions) bool {
	if opts.Collection != "" && !strings.EqualFold(card.Collection, models.Slugify(opts.Collection)) {
		return false
	}
	if opts.Layout != "" && card.Layout != opts.Layout {
		return false
	}
	if opts.Modifier != "" {
		targetMod := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(opts.Modifier), "/"))
		found := false
		for _, m := range card.Modifiers {
			if strings.ToLower(m) == targetMod || strings.HasPrefix(strings.ToLower(m), targetMod) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if len(opts.Tags) > 0 {
		cardTags := make(map[string]struct{}, len(card.Tags))
		for _, t := range card.Tags {
			cardTags[strings.ToLower(t)] = struct{}{}
		}
		for _, req := range opts.Tags {
			req = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(req), "#"))
			if req == "" {
				continue
			}
			if _, ok := cardTags[req]; !ok {
				return false
			}
		}
	}
	return true
}

// tokenizeQuery extracts lowercase alphanumeric/underscore/hyphen tokens for FTS5 and header checks.
func tokenizeQuery(input string) []string {
	fields := strings.FieldsFunc(input, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-')
	})
	if len(fields) == 0 {
		return nil
	}
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.ToLower(strings.Trim(f, "-"))
		if f != "" {
			out = append(out, f)
		}
	}
	return out
}

// buildFTS5QueryFromTokens constructs an FTS5 MATCH expression with either exact tokens or prefix wildcards.
func buildFTS5QueryFromTokens(tokens []string, prefix bool) string {
	if len(tokens) == 0 {
		return ""
	}
	parts := make([]string, 0, len(tokens))
	for _, tok := range tokens {
		if prefix {
			parts = append(parts, `"`+tok+`"*`)
		} else {
			parts = append(parts, `"`+tok+`"`)
		}
	}
	return strings.Join(parts, " ")
}
