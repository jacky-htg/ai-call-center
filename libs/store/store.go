package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	DB *sql.DB
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	s := &Store{DB: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	if s.DB == nil {
		return nil
	}
	return s.DB.Close()
}

func (s *Store) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS users (id TEXT PRIMARY KEY, name TEXT);`,
		`CREATE TABLE IF NOT EXISTS calls (id TEXT PRIMARY KEY, caller_id TEXT, status TEXT, created_at INTEGER);`,
		`CREATE TABLE IF NOT EXISTS sessions (id TEXT PRIMARY KEY, call_id TEXT, user_id TEXT, type TEXT, status TEXT, created_at INTEGER);`,
	}
	for _, q := range stmts {
		if _, err := s.DB.Exec(q); err != nil {
			return err
		}
	}

	// Add token column to sessions if not present (SQLite will error if exists; ignore)
	if _, err := s.DB.Exec(`ALTER TABLE sessions ADD COLUMN token TEXT;`); err != nil {
		// ignore "duplicate column name" or other errors - simple migration strategy
	}
	return nil
}

func genID() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// CreateCall creates a call row and an initial session for the caller. Returns callID and sessionID.
func (s *Store) CreateCall(callerID string) (string, string, error) {
	if callerID == "" {
		return "", "", errors.New("caller_id required")
	}
	callID, err := genID()
	if err != nil {
		return "", "", err
	}
	sessionID, err := genID()
	if err != nil {
		return "", "", err
	}

	tx, err := s.DB.Begin()
	if err != nil {
		return "", "", err
	}
	now := time.Now().Unix()
	if _, err := tx.Exec(`INSERT INTO calls(id, caller_id, status, created_at) VALUES(?,?,?,?)`, callID, callerID, "new", now); err != nil {
		tx.Rollback()
		return "", "", err
	}
	if _, err := tx.Exec(`INSERT INTO sessions(id, call_id, user_id, type, status, created_at) VALUES(?,?,?,?,?,?)`, sessionID, callID, callerID, "caller", "new", now); err != nil {
		tx.Rollback()
		return "", "", err
	}
	if err := tx.Commit(); err != nil {
		return "", "", err
	}
	return callID, sessionID, nil
}

func (s *Store) CreateSession(callID, userID, typ, status string) (string, error) {
	id, err := genID()
	if err != nil {
		return "", err
	}
	now := time.Now().Unix()
	if _, err := s.DB.Exec(`INSERT INTO sessions(id, call_id, user_id, type, status, created_at) VALUES(?,?,?,?,?,?)`, id, callID, userID, typ, status, now); err != nil {
		return "", err
	}
	return id, nil
}

func (s *Store) UpdateSessionStatus(sessionID, status string) error {
	res, err := s.DB.Exec(`UPDATE sessions SET status = ? WHERE id = ?`, status, sessionID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("session not found: %s", sessionID)
	}
	return nil
}

// UpdateSessionToken stores a token (e.g., LiveKit access token) for the session.
func (s *Store) UpdateSessionToken(sessionID, token string) error {
	res, err := s.DB.Exec(`UPDATE sessions SET token = ? WHERE id = ?`, token, sessionID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("session not found: %s", sessionID)
	}
	return nil
}

// GetSessionToken retrieves the stored token for a session.
func (s *Store) GetSessionToken(sessionID string) (string, error) {
	var token sql.NullString
	row := s.DB.QueryRow(`SELECT token FROM sessions WHERE id = ?`, sessionID)
	if err := row.Scan(&token); err != nil {
		return "", err
	}
	if token.Valid {
		return token.String, nil
	}
	return "", nil
}

func (s *Store) UpdateCallStatus(callID, status string) error {
	res, err := s.DB.Exec(`UPDATE calls SET status = ? WHERE id = ?`, status, callID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("call not found: %s", callID)
	}
	return nil
}

func (s *Store) FindSessionByIdentity(identity string) (string, string, error) {
	// identity is session id which maps to sessions.id
	var callID, status string
	row := s.DB.QueryRow(`SELECT call_id, status FROM sessions WHERE id = ?`, identity)
	if err := row.Scan(&callID, &status); err != nil {
		return "", "", err
	}
	return callID, status, nil
}
