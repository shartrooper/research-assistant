package storage

import (
	"database/sql"
	_ "embed"
	"fmt"
	"os"

	_ "github.com/mattn/go-sqlite3"
	"github.com/user/research-assistant/internal/event"
)

//go:embed migrations/000001_initial_schema.up.sql
var schemaSQL string

// StructuredStorage defines the interface for storing structured research data
type StructuredStorage interface {
	CreateSession(id, topic string) error
	UpdateSessionStatus(id, status string, errMsg string) error
	SaveFindings(sessionID string, findings []event.StructuredFinding) error
	SaveOpenQuestions(sessionID string, questions []string) error
	SaveSources(sessionID string, sources []event.SearchSource) error
	MarkSessionComplete(id, reportKey, jsonKey string) error
}

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	_, statErr := os.Stat(dbPath)
	isNew := os.IsNotExist(statErr)

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	if isNew {
		if _, err := db.Exec(schemaSQL); err != nil {
			db.Close()
			return nil, fmt.Errorf("apply schema: %w", err)
		}
	}

	return &SQLiteStore{db: db}, nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func (s *SQLiteStore) CreateSession(id, topic string) error {
	query := `INSERT INTO research_sessions (id, topic, status) VALUES (?, ?, 'queued')`
	_, err := s.db.Exec(query, id, topic)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

func (s *SQLiteStore) UpdateSessionStatus(id, status string, errMsg string) error {
	query := `UPDATE research_sessions SET status = ?, error = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`
	var errStr sql.NullString
	if errMsg != "" {
		errStr.String = errMsg
		errStr.Valid = true
	}
	_, err := s.db.Exec(query, status, errStr, id)
	if err != nil {
		return fmt.Errorf("update session status: %w", err)
	}
	return nil
}

func (s *SQLiteStore) SaveFindings(sessionID string, findings []event.StructuredFinding) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT INTO key_findings (session_id, finding, confidence) VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, f := range findings {
		if _, err := stmt.Exec(sessionID, f.Finding, f.Confidence); err != nil {
			return fmt.Errorf("insert finding: %w", err)
		}
	}

	return tx.Commit()
}

func (s *SQLiteStore) SaveOpenQuestions(sessionID string, questions []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT INTO open_questions (session_id, question) VALUES (?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, q := range questions {
		if _, err := stmt.Exec(sessionID, q); err != nil {
			return fmt.Errorf("insert question: %w", err)
		}
	}

	return tx.Commit()
}

func (s *SQLiteStore) SaveSources(sessionID string, sources []event.SearchSource) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT INTO sources (session_id, query, url, snippet) VALUES (?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, src := range sources {
		if _, err := stmt.Exec(sessionID, src.Query, src.URL, src.Snippet); err != nil {
			return fmt.Errorf("insert source: %w", err)
		}
	}

	return tx.Commit()
}

func (s *SQLiteStore) MarkSessionComplete(id, reportKey, jsonKey string) error {
	query := `UPDATE research_sessions 
              SET status = 'complete', report_md_key = ?, report_json_key = ?, updated_at = CURRENT_TIMESTAMP 
              WHERE id = ?`
	_, err := s.db.Exec(query, reportKey, jsonKey, id)
	if err != nil {
		return fmt.Errorf("mark session complete: %w", err)
	}
	return nil
}

