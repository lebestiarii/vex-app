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
	MatchSource string            `json:"matchSource"` // "fts", "fuzzy", "vector", "hybrid", "filter"
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

func (s *Store) migrate() error {
	stmts := []string{
		`PRAGMA journal_mode = WAL;`,
		`PRAGMA busy_timeout = 5000;`,
		`PRAGMA foreign_keys = ON;`,
		`CREATE TABLE IF NOT EXISTS cards (
			id TEXT PRIMARY KEY,
			collection TEXT NOT NULL,
			template TEXT NOT NULL,
			title TEXT NOT NULL,
			subtitle TEXT NOT NULL DEFAULT '',
			description TEXT NOT NULL DEFAULT '',
			tags_json TEXT NOT NULL DEFAULT '[]',
			modifiers_json TEXT NOT NULL DEFAULT '[]',
			card_json TEXT NOT NULL,
			content_hash TEXT NOT NULL,
			source_file TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_cards_collection ON cards(collection);`,
		`CREATE INDEX IF NOT EXISTS idx_cards_source_file ON cards(source_file);`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS cards_fts USING fts5(
			id UNINDEXED,
			collection UNINDEXED,
			title,
			modifiers,
			tags,
			subtitle,
			body,
			tokenize = 'porter unicode61'
		);`,
		`CREATE TABLE IF NOT EXISTS card_embeddings (
			card_id TEXT PRIMARY KEY,
			model TEXT NOT NULL,
			content_hash TEXT NOT NULL,
			vector BLOB NOT NULL,
			dims INTEGER NOT NULL,
			FOREIGN KEY (card_id) REFERENCES cards(id) ON DELETE CASCADE
		);`,
	}

	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("migrate sqlite (%s): %w", stmt[:min(40, len(stmt))], err)
		}
	}
	return nil
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

	allCards, err := s.ListCards(ctx, "")
	if err != nil {
		return nil, err
	}

	type scoredCard struct {
		card  models.SearchCard
		score int
	}
	var matches []scoredCard
	for _, c := range allCards {
		if c.ID == target.ID {
			continue
		}
		score := 0
		if c.EntityKey != "" {
			if w, ok := needles[strings.ToLower(c.EntityKey)]; ok {
				score += w * 2
			}
		}
		for _, rk := range c.RelatedKeys {
			if w, ok := needles[strings.ToLower(rk)]; ok {
				score += w
			}
		}
		for _, t := range c.Tags {
			if w, ok := needles[strings.ToLower(t)]; ok {
				score += w
			}
		}
		if score >= 5 {
			matches = append(matches, scoredCard{card: c, score: score})
		}
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		return matches[i].card.Title < matches[j].card.Title
	})

	if len(matches) > limit {
		matches = matches[:limit]
	}
	out := make([]models.SearchCard, len(matches))
	for i, m := range matches {
		out[i] = m.card
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
	tagsJSON, _ := json.Marshal(card.Tags)
	modsJSON, _ := json.Marshal(card.Modifiers)
	cardJSON, err := json.Marshal(card)
	if err != nil {
		return err
	}
	hash := card.ContentHash()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO cards (id, collection, template, title, subtitle, description, tags_json, modifiers_json, card_json, content_hash, source_file, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			collection = excluded.collection,
			template = excluded.template,
			title = excluded.title,
			subtitle = excluded.subtitle,
			description = excluded.description,
			tags_json = excluded.tags_json,
			modifiers_json = excluded.modifiers_json,
			card_json = excluded.card_json,
			content_hash = excluded.content_hash,
			source_file = excluded.source_file,
			updated_at = excluded.updated_at
	`,
		card.ID, card.Collection, string(card.Template), card.Title, card.Subtitle,
		card.Description, string(tagsJSON), string(modsJSON), string(cardJSON),
		hash, sourceFile, card.UpdatedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return err
	}

	// Keep FTS5 row synchronized.
	if _, err := tx.ExecContext(ctx, `DELETE FROM cards_fts WHERE id = ?`, card.ID); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO cards_fts (id, collection, title, modifiers, tags, subtitle, body)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`,
		card.ID,
		card.Collection,
		card.Title,
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

// SearchHybrid executes a combined FTS5 BM25 + substring fuzzy + Vector Cosine RRF search.
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

	type rrfEntry struct {
		card      models.SearchCard
		score     float64
		hasFTS    bool
		hasFuzzy  bool
		hasVector bool
	}

	const rrfK = 60.0
	merged := make(map[string]*rrfEntry)
	addRank := func(card models.SearchCard, rank int, weight float64, source string) {
		entry, ok := merged[card.ID]
		if !ok {
			entry = &rrfEntry{card: card}
			merged[card.ID] = entry
		}
		entry.score += weight / (rrfK + float64(rank+1))
		switch source {
		case "fts":
			entry.hasFTS = true
		case "fuzzy":
			entry.hasFuzzy = true
		case "vector":
			entry.hasVector = true
		}
	}

	candidateLimit := limit * 3
	if candidateLimit < 30 {
		candidateLimit = 30
	}

	// 1. FTS5 BM25 Lexical Search
	if query != "" {
		ftsQuery := buildFTS5Query(query)
		if ftsQuery != "" {
			ftsCards, err := s.searchFTSLocked(ctx, ftsQuery, opts, candidateLimit)
			if err == nil {
				for rank, card := range ftsCards {
					addRank(card, rank, 1.0, "fts")
				}
			}
		}

		// 2. Substring / Fuzzy Fallback (handles partial URLs, snake_case, paths)
		if len(merged) < limit {
			fuzzyCards, err := s.searchFuzzyLocked(ctx, query, opts, candidateLimit)
			if err == nil {
				for rank, card := range fuzzyCards {
					addRank(card, rank, 0.55, "fuzzy")
				}
			}
		}
	}

	// 3. Vector Cosine Similarity Search (when query embedding is provided)
	if len(opts.QueryEmbedding) > 0 {
		vecCards, err := s.searchVectorLocked(ctx, opts.QueryEmbedding, opts, candidateLimit)
		if err == nil {
			for rank, card := range vecCards {
				addRank(card, rank, 1.1, "vector")
			}
		}
	}

	results := make([]SearchResult, 0, len(merged))
	for _, e := range merged {
		source := "fts"
		switch {
		case (e.hasFTS || e.hasFuzzy) && e.hasVector:
			source = "hybrid"
		case e.hasVector:
			source = "vector"
		case e.hasFTS:
			source = "fts"
		case e.hasFuzzy:
			source = "fuzzy"
		}
		results = append(results, SearchResult{
			Card:        e.card,
			Score:       e.score,
			MatchSource: source,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return results[i].Card.Title < results[j].Card.Title
	})

	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
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

func (s *Store) searchFTSLocked(ctx context.Context, ftsQuery string, opts SearchOptions, limit int) ([]models.SearchCard, error) {
	// Weighted BM25: id(0), collection(0), title(10.0), modifiers(8.0), tags(8.0), subtitle(5.0), body(2.0)
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.card_json
		FROM cards_fts f
		JOIN cards c ON c.id = f.id
		WHERE cards_fts MATCH ?
		ORDER BY bm25(cards_fts, 0.0, 0.0, 10.0, 8.0, 8.0, 5.0, 2.0) ASC
		LIMIT ?
	`, ftsQuery, limit*2)
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

func (s *Store) searchFuzzyLocked(ctx context.Context, query string, opts SearchOptions, limit int) ([]models.SearchCard, error) {
	pattern := "%" + strings.ToLower(query) + "%"
	rows, err := s.db.QueryContext(ctx, `
		SELECT card_json
		FROM cards
		WHERE lower(title) LIKE ? OR lower(subtitle) LIKE ? OR lower(description) LIKE ? OR lower(card_json) LIKE ?
		ORDER BY CASE WHEN lower(title) LIKE ? THEN 0 ELSE 1 END, title ASC
		LIMIT ?
	`, pattern, pattern, pattern, pattern, pattern, limit*2)
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

func (s *Store) searchVectorLocked(ctx context.Context, queryVec []float32, opts SearchOptions, limit int) ([]models.SearchCard, error) {
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

	type scoredCard struct {
		card models.SearchCard
		sim  float64
	}
	var scored []scoredCard
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
			scored = append(scored, scoredCard{card: card, sim: sim})
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

	out := make([]models.SearchCard, len(scored))
	for i, sc := range scored {
		out[i] = sc.card
	}
	return out, nil
}

// FindCardsByEntityKeys returns indexed SearchCards whose EntityKey matches any of the requested keys (case-insensitive).
func (s *Store) FindCardsByEntityKeys(ctx context.Context, entityKeys []string) ([]models.SearchCard, error) {
	want := make(map[string]struct{}, len(entityKeys))
	for _, k := range entityKeys {
		clean := strings.ToLower(strings.TrimSpace(k))
		if clean != "" {
			want[clean] = struct{}{}
		}
	}
	if len(want) == 0 {
		return nil, nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.QueryContext(ctx, `SELECT card_json FROM cards`)
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
		if _, ok := want[strings.ToLower(strings.TrimSpace(card.EntityKey))]; ok {
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

// buildFTS5Query safely tokenizes user text into prefix-matched FTS5 terms.
func buildFTS5Query(input string) string {
	fields := strings.FieldsFunc(input, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-')
	})
	if len(fields) == 0 {
		return ""
	}
	parts := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.Trim(f, "-")
		if f == "" {
			continue
		}
		parts = append(parts, `"`+f+`"*`)
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
