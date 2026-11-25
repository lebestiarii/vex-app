package indexer

import (
	"context"
	"database/sql"
	"encoding/binary"
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

// SearchOptions configures a hybrid lexical + semantic search query.
type SearchOptions struct {
	Query          string                  `json:"query"`
	Collection     string                  `json:"collection,omitempty"`
	Tags           []string                `json:"tags,omitempty"`
	Modifier       string                  `json:"modifier,omitempty"`
	Template       models.CardTemplateType `json:"template,omitempty"`
	Limit          int                     `json:"limit,omitempty"`
	QueryEmbedding []float32               `json:"-"`
	EmbeddingModel string                  `json:"-"`
}

// SearchResult pairs a matched SearchCard with ranking metadata.
type SearchResult struct {
	Card        models.SearchCard `json:"card"`
	Score       float64           `json:"score"`
	MatchSource string            `json:"matchSource"` // "exact", "contextual", "fts", "fuzzy", "vector", "hybrid", "filter"
}

// Store manages the embedded SQLite FTS5 + Vector index.
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

const currentSchemaVersion = 2

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
		template TEXT NOT NULL,
		title TEXT NOT NULL,
		subtitle TEXT NOT NULL DEFAULT '',
		description TEXT NOT NULL DEFAULT '',
		entity_key TEXT NOT NULL DEFAULT '',
		entity_code TEXT NOT NULL DEFAULT '',
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

	const createEmbeddingsTableSQL = `CREATE TABLE IF NOT EXISTS card_embeddings (
		card_id TEXT PRIMARY KEY,
		model TEXT NOT NULL,
		content_hash TEXT NOT NULL,
		vector BLOB NOT NULL,
		dims INTEGER NOT NULL,
		FOREIGN KEY (card_id) REFERENCES cards(id) ON DELETE CASCADE
	);`

	indexStmts := []string{
		`CREATE INDEX IF NOT EXISTS idx_cards_collection ON cards(collection);`,
		`CREATE INDEX IF NOT EXISTS idx_cards_source_file ON cards(source_file);`,
		`CREATE INDEX IF NOT EXISTS idx_cards_entity_key ON cards(entity_key);`,
		`CREATE INDEX IF NOT EXISTS idx_cards_entity_code ON cards(entity_code);`,
		`CREATE INDEX IF NOT EXISTS idx_cards_updated_at ON cards(updated_at DESC);`,
	}

	// Fast path 1: Already at current schema version.
	if cardsTableCount > 0 && userVersion >= currentSchemaVersion {
		for _, stmt := range append([]string{createEmbeddingsTableSQL}, indexStmts...) {
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

	// Fast path 2: Brand-new database — initialize directly at schema v2.
	if cardsTableCount == 0 {
		initStmts := append(
			[]string{createCardsTableSQL, createFTSTableSQL, createEmbeddingsTableSQL},
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

	// Upgrade path: Legacy v0/v1 database -> atomic in-place upgrade to v2 with full row backfill.
	existingCols := make(map[string]bool)
	colRows, err := tx.Query(`PRAGMA table_info(cards);`)
	if err != nil {
		return fmt.Errorf("inspect cards columns: %w", err)
	}
	for colRows.Next() {
		var (
			cid      int
			name     string
			colType  string
			notNull  int
			dfltVal  sql.NullString
			pk       int
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

	requiredCols := []struct {
		name string
		ddl  string
	}{
		{"entity_key", `ALTER TABLE cards ADD COLUMN entity_key TEXT NOT NULL DEFAULT '';`},
		{"entity_code", `ALTER TABLE cards ADD COLUMN entity_code TEXT NOT NULL DEFAULT '';`},
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

	if _, err := tx.Exec(createEmbeddingsTableSQL); err != nil {
		return fmt.Errorf("ensure card_embeddings: %w", err)
	}
	for _, stmt := range indexStmts {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("create sqlite index (%s): %w", stmt[:min(40, len(stmt))], err)
		}
	}

	// Rebuild cards_fts with the 9-column schema.
	if _, err := tx.Exec(`DROP TABLE IF EXISTS cards_fts;`); err != nil {
		return fmt.Errorf("drop legacy cards_fts: %w", err)
	}
	if _, err := tx.Exec(createFTSTableSQL); err != nil {
		return fmt.Errorf("create v2 cards_fts: %w", err)
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
			chipsText := card.ChipsText()
			relatedKeysJSON := marshalStringSliceJSON(card.RelatedKeys)
			if _, err := tx.Exec(`
				UPDATE cards
				SET entity_key = ?, entity_code = ?, chips_text = ?, related_keys_json = ?
				WHERE id = ?
			`, entityKey, entityCode, chipsText, relatedKeysJSON, r.id); err != nil {
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
		} else {
			// Defensive fallback if a legacy row had malformed JSON: preserve its scalar text in FTS.
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

// UpsertCard inserts or updates a single card in both the main table and FTS5 index.
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

	// Find existing cards for this sourceFile (or collection if sourceFile is empty) to prune removed ones.
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

	// Fetch card_json only for the top-K winning IDs.
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
	defer cardRows.Close()

	byID := make(map[string]models.SearchCard, len(matches))
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

	out := make([]models.SearchCard, 0, len(matches))
	for _, m := range matches {
		if card, ok := byID[m.id]; ok {
			out = append(out, card)
		}
	}
	return out, nil
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
	chipsText := card.ChipsText()

	cardJSON, err := json.Marshal(card)
	if err != nil {
		return err
	}
	hash := card.ContentHash()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO cards (
			id, collection, template, title, subtitle, description,
			entity_key, entity_code, chips_text, related_keys_json,
			tags_json, modifiers_json, card_json, content_hash, source_file, updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			collection = excluded.collection,
			template = excluded.template,
			title = excluded.title,
			subtitle = excluded.subtitle,
			description = excluded.description,
			entity_key = excluded.entity_key,
			entity_code = excluded.entity_code,
			chips_text = excluded.chips_text,
			related_keys_json = excluded.related_keys_json,
			tags_json = excluded.tags_json,
			modifiers_json = excluded.modifiers_json,
			card_json = excluded.card_json,
			content_hash = excluded.content_hash,
			source_file = excluded.source_file,
			updated_at = excluded.updated_at
	`,
		card.ID, card.Collection, string(card.Template), card.Title, card.Subtitle, card.Description,
		entityKey, entityCode, chipsText, relatedKeysJSON,
		tagsJSON, modsJSON, string(cardJSON), hash, sourceFile, card.UpdatedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return err
	}

	// Keep 9-column FTS5 row synchronized.
	if _, err := tx.ExecContext(ctx, `DELETE FROM cards_fts WHERE id = ?`, card.ID); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
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
	)
	return err
}

// GetCard retrieves a card by its ID.
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
	return &card, nil
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

// UpsertEmbedding stores a normalized vector embedding for a card.
func (s *Store) UpsertEmbedding(ctx context.Context, cardID, model, contentHash string, vec []float32) error {
	if len(vec) == 0 {
		return errors.New("embedding vector is empty")
	}
	blob := encodeFloat32Vector(normalizeVector(vec))

	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO card_embeddings (card_id, model, content_hash, vector, dims)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(card_id) DO UPDATE SET
			model = excluded.model,
			content_hash = excluded.content_hash,
			vector = excluded.vector,
			dims = excluded.dims
	`, cardID, model, contentHash, blob, len(vec))
	return err
}

// CardsNeedingEmbedding returns cards that either have no embedding or whose content_hash changed.
func (s *Store) CardsNeedingEmbedding(ctx context.Context, model string) ([]models.SearchCard, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.QueryContext(ctx, `
		SELECT c.card_json
		FROM cards c
		LEFT JOIN card_embeddings e ON c.id = e.card_id AND e.model = ?
		WHERE e.card_id IS NULL OR e.content_hash != c.content_hash
	`, model)
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

// SearchHybrid executes a 4-tier exact/contextual/prefix/guarded-fuzzy lexical search
// combined with optional Vector Cosine similarity for hybrid RAG retrieval.
func (s *Store) SearchHybrid(ctx context.Context, opts SearchOptions) ([]SearchResult, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}
	query := strings.TrimSpace(opts.Query)

	s.mu.RLock()
	defer s.mu.RUnlock()

	// If no text query and no embedding, return filtered cards sorted by updated_at desc.
	if query == "" && len(opts.QueryEmbedding) == 0 {
		return s.searchFilterOnlyLocked(ctx, opts, limit)
	}

	type rankEntry struct {
		card          models.SearchCard
		lexicalScore  float64
		lexicalTier   int    // 1=exact, 2=fts-exact/contextual, 3=fts-prefix, 4=fuzzy
		lexicalSource string // "exact", "contextual", "fts", "fuzzy"
		vectorSim     float64
		vectorRank    int
		hasVector     bool
	}

	merged := make(map[string]*rankEntry)
	recordLexical := func(card models.SearchCard, score float64, tier int, source string) {
		entry, ok := merged[card.ID]
		if !ok {
			entry = &rankEntry{card: card, vectorRank: -1}
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

	if query != "" {
		tokens := tokenizeQuery(query)
		exactFTSQuery := buildFTS5QueryFromTokens(tokens, false)
		prefixFTSQuery := buildFTS5QueryFromTokens(tokens, true)

		// Tier 1 (80.0 - 100.0): Exact ID, entity_key, entity_code, title, modifier, tag, chip, or (CODE) match.
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
							// Never demote a Tier 1 exact match.
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
						// Body-only prefix hits stay below the 0.25*topScore cliff when Tier 1 exists.
						score = 16.0 + 8.0*combined
					}
					recordLexical(hit.card, score, 3, "fts")
				}
			}
		}

		// Tier 4 (5.0 - 14.0): Guarded metadata-only substring fallback (NEVER scans card_json).
		// Runs only when Tiers 1-3 yielded fewer than 3 hits and query has at least 3 runes.
		if len(merged) < 3 && len([]rune(query)) >= 3 {
			fuzzyCards, err := s.searchFuzzyLocked(ctx, query, opts, candidateLimit)
			if err == nil {
				for rank, card := range fuzzyCards {
					score := 14.0 - math.Min(9.0, float64(rank)*1.2)
					recordLexical(card, score, 4, "fuzzy")
				}
			}
		}
	}

	// Optional Vector Cosine Similarity Search (for AI RAG / semantic hybrid mode).
	if len(opts.QueryEmbedding) > 0 {
		vecHits, err := s.searchVectorLocked(ctx, opts.QueryEmbedding, opts, candidateLimit)
		if err == nil {
			for rank, hit := range vecHits {
				entry, ok := merged[hit.card.ID]
				if !ok {
					entry = &rankEntry{card: hit.card, vectorRank: -1}
					merged[hit.card.ID] = entry
				}
				entry.hasVector = true
				entry.vectorSim = hit.sim
				entry.vectorRank = rank
			}
		}
	}

	results := make([]SearchResult, 0, len(merged))
	for _, e := range merged {
		score := e.lexicalScore
		source := e.lexicalSource
		if e.hasVector {
			vecScore := 40.0*e.vectorSim + 15.0/(1.0+0.15*float64(e.vectorRank))
			if e.lexicalTier > 0 {
				score = e.lexicalScore + vecScore
				source = "hybrid"
			} else {
				score = vecScore
				source = "vector"
			}
		}
		if source == "" {
			source = "fts"
		}
		results = append(results, SearchResult{
			Card:        e.card,
			Score:       score,
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
	if query != "" && len(opts.QueryEmbedding) == 0 && len(results) > 1 {
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

	// Pass 2: Column-scoped FTS5 exact-token lookup across structured metadata columns
	// (entity_key, title, chips, modifiers, tags), then verified unstemmed in Go.
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
		SELECT id, title, entity_key, related_keys_json
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
		var id, title, entityKey, relatedRaw string
		if err := rows.Scan(&id, &title, &entityKey, &relatedRaw); err != nil {
			rows.Close()
			return nil, err
		}
		if _, isSeed := seedIDs[id]; isSeed {
			continue
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

	// Fetch card_json only for matched neighbor IDs (cap fetch pool to stay lightweight).
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
	rows, err := s.db.QueryContext(ctx, `SELECT card_json FROM cards ORDER BY updated_at DESC, title ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]SearchResult, 0, limit)
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
		out = append(out, SearchResult{
			Card:        card,
			Score:       1.0,
			MatchSource: "filter",
		})
		if len(out) >= limit {
			break
		}
	}
	return out, rows.Err()
}

type ftsCandidate struct {
	card models.SearchCard
	bm25 float64
}

func (s *Store) searchFTSLocked(ctx context.Context, ftsQuery string, opts SearchOptions, limit int) ([]ftsCandidate, error) {
	// Weighted BM25 across 9 columns:
	// id(0), collection(0), entity_key(14.0), title(12.0), chips(10.0), modifiers(9.0), tags(9.0), subtitle(6.0), body(2.0)
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

type vectorCandidate struct {
	card models.SearchCard
	sim  float64
}

func (s *Store) searchVectorLocked(ctx context.Context, queryVec []float32, opts SearchOptions, limit int) ([]vectorCandidate, error) {
	qNorm := normalizeVector(queryVec)
	var rows *sql.Rows
	var err error
	if opts.EmbeddingModel != "" {
		rows, err = s.db.QueryContext(ctx, `
			SELECT c.card_json, e.vector, e.dims
			FROM card_embeddings e
			JOIN cards c ON c.id = e.card_id
			WHERE e.model = ?
		`, opts.EmbeddingModel)
	} else {
		rows, err = s.db.QueryContext(ctx, `
			SELECT c.card_json, e.vector, e.dims
			FROM card_embeddings e
			JOIN cards c ON c.id = e.card_id
		`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var scored []vectorCandidate
	for rows.Next() {
		var raw string
		var blob []byte
		var dims int
		if err := rows.Scan(&raw, &blob, &dims); err != nil {
			return nil, err
		}
		if dims != len(qNorm) {
			continue
		}
		var card models.SearchCard
		if err := json.Unmarshal([]byte(raw), &card); err != nil {
			continue
		}
		if !matchesFilters(card, opts) {
			continue
		}
		vec := decodeFloat32Vector(blob, dims)
		sim := dotProduct(qNorm, vec)
		if sim > 0.05 {
			scored = append(scored, vectorCandidate{card: card, sim: sim})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].sim > scored[j].sim
	})
	if len(scored) > limit {
		scored = scored[:limit]
	}
	return scored, nil
}

// FindCardsByEntityKeys returns indexed SearchCards whose EntityKey (or EntityCode when given a bare code)
// matches any of the requested keys (case-insensitive) using B-tree indexes.
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

func matchesFilters(card models.SearchCard, opts SearchOptions) bool {
	if opts.Collection != "" && !strings.EqualFold(card.Collection, models.Slugify(opts.Collection)) {
		return false
	}
	if opts.Template != "" && card.Template != opts.Template {
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

func normalizeVector(v []float32) []float32 {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	norm := math.Sqrt(sum)
	out := make([]float32, len(v))
	if norm == 0 {
		return out
	}
	for i, x := range v {
		out[i] = float32(float64(x) / norm)
	}
	return out
}

func dotProduct(a, b []float32) float64 {
	n := min(len(a), len(b))
	var sum float64
	for i := 0; i < n; i++ {
		sum += float64(a[i]) * float64(b[i])
	}
	return sum
}

func encodeFloat32Vector(v []float32) []byte {
	buf := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(buf[i*4:(i+1)*4], math.Float32bits(f))
	}
	return buf
}

func decodeFloat32Vector(b []byte, dims int) []float32 {
	out := make([]float32, dims)
	for i := 0; i < dims && (i+1)*4 <= len(b); i++ {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4 : (i+1)*4]))
	}
	return out
}
