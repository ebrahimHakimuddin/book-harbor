// Package audit records administrator actions that change or remove data.
package audit

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type Store struct{ db *sql.DB }

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

type Entry struct {
	ID         int64     `json:"id"`
	ActorID    string    `json:"actorId"`
	ActorEmail string    `json:"actorEmail"`
	Action     string    `json:"action"`
	TargetType string    `json:"targetType"`
	TargetID   string    `json:"targetId"`
	Summary    string    `json:"summary"`
	CreatedAt  time.Time `json:"createdAt"`
}

// Record stores one entry. Summaries must not contain secrets.
func (s *Store) Record(ctx context.Context, e Entry) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO audit_log (actor_id, actor_email, action, target_type, target_id, summary, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		e.ActorID, e.ActorEmail, e.Action, e.TargetType, e.TargetID, e.Summary, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("record audit entry: %w", err)
	}
	return nil
}

// List returns the newest entries first.
func (s *Store) List(ctx context.Context, limit int) ([]Entry, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, actor_id, actor_email, action, target_type, target_id, summary, created_at
		FROM audit_log ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("list audit entries: %w", err)
	}
	defer rows.Close()
	entries := make([]Entry, 0)
	for rows.Next() {
		var e Entry
		var createdAt string
		if err := rows.Scan(&e.ID, &e.ActorID, &e.ActorEmail, &e.Action, &e.TargetType, &e.TargetID, &e.Summary, &createdAt); err != nil {
			return nil, fmt.Errorf("scan audit entry: %w", err)
		}
		if e.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
			return nil, fmt.Errorf("parse audit time: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
