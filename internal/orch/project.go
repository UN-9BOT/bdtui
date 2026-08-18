package orch

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
)

func (s *Store) CreateProject(ctx context.Context, p *Project) error {
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	now := nowUTC()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	p.UpdatedAt = now

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO projects(id, name, fs_path, git_remote, created_at, updated_at)
		 VALUES(?, ?, ?, ?, ?, ?)`,
		p.ID, p.Name, p.FsPath, p.GitRemote, timeString(p.CreatedAt), timeString(p.UpdatedAt),
	); err != nil {
		return err
	}
	if err := appendEventMapTx(ctx, tx, nil, EventProjectUpserted, map[string]any{"project_id": p.ID}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) GetProject(ctx context.Context, id string) (*Project, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, fs_path, git_remote, created_at, updated_at FROM projects WHERE id = ?`, id)
	p := &Project{}
	var created, updated string
	if err := row.Scan(&p.ID, &p.Name, &p.FsPath, &p.GitRemote, &created, &updated); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var err error
	if p.CreatedAt, err = parseTime(created); err != nil {
		return nil, err
	}
	if p.UpdatedAt, err = parseTime(updated); err != nil {
		return nil, err
	}
	return p, nil
}

// UpdateProject mutates the project's movable attributes (name, fs_path,
// git_remote). The ID is immutable.
func (s *Store) UpdateProject(ctx context.Context, p *Project) error {
	p.UpdatedAt = nowUTC()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		`UPDATE projects SET name = ?, fs_path = ?, git_remote = ?, updated_at = ? WHERE id = ?`,
		p.Name, p.FsPath, p.GitRemote, timeString(p.UpdatedAt), p.ID,
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}

	if err := appendEventMapTx(ctx, tx, nil, EventProjectUpserted, map[string]any{
		"project_id": p.ID,
	}); err != nil {
		return err
	}
	return tx.Commit()
}
