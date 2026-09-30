package pgstore

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"colorbook/internal/essay"
)

const migrate = `
CREATE TABLE IF NOT EXISTS days (
  day date PRIMARY KEY,
  sealed boolean NOT NULL DEFAULT false
);
CREATE TABLE IF NOT EXISTS slots (
  day date NOT NULL REFERENCES days(day) ON DELETE CASCADE,
  round_number bigint NOT NULL,
  word text NOT NULL,
  PRIMARY KEY (day, round_number)
);`

type Store struct {
	DB *pgxpool.Pool
}

type Essay struct {
	Day       string   `json:"day"`
	Sealed    bool     `json:"sealed"`
	Fragments []string `json:"fragments"`
	Text      string   `json:"text"`
	Found     bool     `json:"-"`
}

func Open(ctx context.Context) (*Store, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://colorbook:colorbook@localhost:5432/colorbook?sslmode=disable"
	}
	var last error
	for i := 0; i < 30; i++ {
		pool, err := pgxpool.New(ctx, dsn)
		if err == nil {
			if _, err = pool.Exec(ctx, migrate); err == nil {
				return &Store{DB: pool}, nil
			}
			pool.Close()
		}
		last = err
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return nil, last
}

func (s *Store) Append(ctx context.Context, day string, round int64, word string) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `INSERT INTO days(day, sealed) VALUES ($1, false) ON CONFLICT DO NOTHING`, day); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO slots(day, round_number, word) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, day, round, word); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) Get(ctx context.Context, day string) (Essay, error) {
	out := Essay{Day: day, Fragments: []string{}}
	var sealed bool
	err := s.DB.QueryRow(ctx, `SELECT sealed FROM days WHERE day = $1`, day).Scan(&sealed)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return Essay{}, err
	}
	out.Found = true
	out.Sealed = sealed
	rows, err := s.DB.Query(ctx, `SELECT word FROM slots WHERE day = $1 ORDER BY round_number`, day)
	if err != nil {
		return Essay{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var w string
		if err := rows.Scan(&w); err != nil {
			return Essay{}, err
		}
		out.Fragments = append(out.Fragments, w)
	}
	out.Text = essay.Join(out.Fragments)
	return out, rows.Err()
}

// DaysWithWords lists dates that have at least one winning word, oldest first.
// oldest is inclusive, YYYY-MM-DD. Days with no word are omitted.
func (s *Store) DaysWithWords(ctx context.Context, oldest string) ([]string, error) {
	rows, err := s.DB.Query(ctx, `SELECT DISTINCT day::text FROM slots WHERE day >= $1::date ORDER BY day`, oldest)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var day string
		if err := rows.Scan(&day); err != nil {
			return nil, err
		}
		out = append(out, day)
	}
	return out, rows.Err()
}

// Seal marks day closed and deletes days older than 31 relative to the following UTC day.
func (s *Store) Seal(ctx context.Context, day string) error {
	if _, err := time.Parse(time.DateOnly, day); err != nil {
		return err
	}
	if _, err := s.DB.Exec(ctx, `UPDATE days SET sealed = true WHERE day = $1`, day); err != nil {
		return err
	}
	_, err := s.DB.Exec(ctx, `DELETE FROM slots WHERE day < $1::date - INTERVAL '30 days'`, day)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(ctx, `DELETE FROM days WHERE day < $1::date - INTERVAL '30 days'`, day)
	return err
}
