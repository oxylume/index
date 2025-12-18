package db

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type CrawlerStore struct {
	db *pgxpool.Pool
}

func NewCrawlerStore(db *pgxpool.Pool) *CrawlerStore {
	return &CrawlerStore{
		db: db,
	}
}

func (r *CrawlerStore) GetOffset(ctx context.Context, dns string) (int, error) {
	const sql = `
	select last_offset from crawler_state
	where dns = $1
	`
	offset := 0
	err := r.db.QueryRow(ctx, sql, dns).Scan(&offset)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	return offset, err
}

func (r *CrawlerStore) SetOffset(ctx context.Context, dns string, offset int) error {
	const sql = `
	insert into crawler_state (dns, last_offset)
	values ($1, $2)
	on conflict (dns) do update set
		last_offset = excluded.last_offset
	`
	_, err := r.db.Exec(ctx, sql, dns, offset)
	return err
}
