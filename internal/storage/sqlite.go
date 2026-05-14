package storage

import (
	"database/sql"
	_ "embed"
	"errors"
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
	MarkSessionComplete(id, reportKey, jsonKey, summary string) error
	GetSessionStatus(id string) (status string, errMsg string, err error)
	GetSessionArtifacts(id string) (reportMDKey, reportJSONKey string, err error)
	DeleteSession(id string) error
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

	// Schema uses IF NOT EXISTS so this is safe to run on existing databases.
	_ = isNew
	if _, err := db.Exec(schemaSQL); err != nil {
		err := db.Close()
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("apply schema: %w", err)
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
	defer func(tx *sql.Tx) {
		err := tx.Rollback()
		if err != nil {

		}
	}(tx)

	stmt, err := tx.Prepare(`INSERT INTO key_findings (session_id, finding, confidence) VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}
	defer func(stmt *sql.Stmt) {
		err := stmt.Close()
		if err != nil {

		}
	}(stmt)

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
	defer func(tx *sql.Tx) {
		err := tx.Rollback()
		if err != nil {

		}
	}(tx)

	stmt, err := tx.Prepare(`INSERT INTO open_questions (session_id, question) VALUES (?, ?)`)
	if err != nil {
		return err
	}
	defer func(stmt *sql.Stmt) {
		err := stmt.Close()
		if err != nil {

		}
	}(stmt)

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
	defer func(tx *sql.Tx) {
		err := tx.Rollback()
		if err != nil {

		}
	}(tx)

	stmt, err := tx.Prepare(`INSERT INTO sources (session_id, query, url, snippet) VALUES (?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer func(stmt *sql.Stmt) {
		err := stmt.Close()
		if err != nil {

		}
	}(stmt)

	for _, src := range sources {
		if _, err := stmt.Exec(sessionID, src.Query, src.URL, src.Snippet); err != nil {
			return fmt.Errorf("insert source: %w", err)
		}
	}

	return tx.Commit()
}

// GetKeyFindings retrieves all key findings for the given session.
func (s *SQLiteStore) GetKeyFindings(sessionID string) ([]event.StructuredFinding, error) {
	rows, err := s.db.Query(
		`SELECT finding, confidence FROM key_findings WHERE session_id = ? ORDER BY id`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("query key_findings: %w", err)
	}
	defer func(rows *sql.Rows) {
		err := rows.Close()
		if err != nil {

		}
	}(rows)

	var findings []event.StructuredFinding
	for rows.Next() {
		var f event.StructuredFinding
		if err := rows.Scan(&f.Finding, &f.Confidence); err != nil {
			return nil, fmt.Errorf("scan finding: %w", err)
		}
		findings = append(findings, f)
	}
	return findings, rows.Err()
}

// GetSources retrieves all sources for the given session.
func (s *SQLiteStore) GetSources(sessionID string) ([]event.SearchSource, error) {
	rows, err := s.db.Query(
		`SELECT query, url, COALESCE(snippet, '') FROM sources WHERE session_id = ? ORDER BY id`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("query sources: %w", err)
	}
	defer func(rows *sql.Rows) {
		err := rows.Close()
		if err != nil {

		}
	}(rows)

	var sources []event.SearchSource
	for rows.Next() {
		var src event.SearchSource
		if err := rows.Scan(&src.Query, &src.URL, &src.Snippet); err != nil {
			return nil, fmt.Errorf("scan source: %w", err)
		}
		sources = append(sources, src)
	}
	return sources, rows.Err()
}

func (s *SQLiteStore) MarkSessionComplete(id, reportKey, jsonKey, summary string) error {
	query := `UPDATE research_sessions 
              SET status = 'complete', report_md_key = ?, report_json_key = ?, summary = ?, updated_at = CURRENT_TIMESTAMP 
              WHERE id = ?`
	_, err := s.db.Exec(query, reportKey, jsonKey, summary, id)
	if err != nil {
		return fmt.Errorf("mark session complete: %w", err)
	}
	return nil
}

func (s *SQLiteStore) GetSessionStatus(id string) (string, string, error) {
	query := `SELECT status, COALESCE(error, '') FROM research_sessions WHERE id = ?`
	var status, errMsg string
	err := s.db.QueryRow(query, id).Scan(&status, &errMsg)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", "", nil
		}
		return "", "", fmt.Errorf("get session status: %w", err)
	}
	return status, errMsg, nil
}

func (s *SQLiteStore) GetSessionArtifacts(id string) (string, string, error) {
	query := `SELECT COALESCE(report_md_key, ''), COALESCE(report_json_key, '') FROM research_sessions WHERE id = ?`
	var md, json string
	err := s.db.QueryRow(query, id).Scan(&md, &json)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", "", nil
		}
		return "", "", fmt.Errorf("get session artifacts: %w", err)
	}
	return md, json, nil
}

func (s *SQLiteStore) DeleteSession(id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}

	defer func(tx *sql.Tx) {
		rollbackErr := tx.Rollback()
		if rollbackErr != nil && err == nil {
			err = rollbackErr
		}
	}(tx)

	// Delete from child tables (though CASCADE would be better if we had it in schema)
	tables := []string{"key_findings", "open_questions", "sources"}
	for _, table := range tables {
		if _, err := tx.Exec(fmt.Sprintf("DELETE FROM %s WHERE session_id = ?", table), id); err != nil {
			return fmt.Errorf("delete from %s: %w", table, err)
		}
	}

	// Delete from main table
	if _, err := tx.Exec("DELETE FROM research_sessions WHERE id = ?", id); err != nil {
		return fmt.Errorf("delete from research_sessions: %w", err)
	}

	return tx.Commit()
}
