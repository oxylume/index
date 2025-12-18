package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Stats struct {
	TotalDomains int
	TotalSites   int
	ActiveSites  int
}

type SiteStatus int

const (
	StatusNoSite SiteStatus = iota
	StatusInaccessible
	StatusAccessible
)

type SortBy string

const (
	SortByDomain    SortBy = "domain"
	SortByCheckedAt SortBy = "checked_at"
)

type ListFilters struct {
	Search       string
	Inaccessible bool
	Punycode     *bool
	Spam         bool
	Zone         string

	SortBy SortBy
	Desc   bool
}

type SiteCreate struct {
	Domain  string
	Unicode string
	Zone    string
	Address string
}

type Site struct {
	Domain      string
	Unicode     string
	Address     string
	Status      SiteStatus
	InStorage   bool
	SpamContent bool
	CheckedAt   time.Time
}

type Cursor struct {
	Value  any
	Domain string
}

type SitesStore struct {
	db *pgxpool.Pool
}

func NewSitesStore(db *pgxpool.Pool) *SitesStore {
	return &SitesStore{
		db: db,
	}
}

func (r *SitesStore) GetStats(ctx context.Context) (*Stats, error) {
	const sql = `
	select 
		count(*) as total,
		count(*) filter (where status != $1) as has_sites,
		count(*) filter (where status = $2) as active
	from sites
	`
	var total, sites, activeSites int
	err := r.db.QueryRow(ctx, sql, StatusNoSite, StatusAccessible).Scan(&total, &sites, &activeSites)
	if err != nil {
		return nil, err
	}
	return &Stats{
		TotalDomains: total,
		TotalSites:   sites,
		ActiveSites:  activeSites,
	}, nil
}

func (r *SitesStore) GetRandomSite(ctx context.Context) (*Site, error) {
	const sql = `
	select domain, unicode, address, status, in_storage, spam_content, checked_at from sites
	where status = $1 and spam_content = false
	order by random()
	limit 1
	`
	var s Site
	err := r.db.QueryRow(ctx, sql, StatusAccessible).Scan(
		&s.Domain, &s.Unicode, &s.Address, &s.Status, &s.InStorage, &s.SpamContent, &s.CheckedAt)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *SitesStore) List(ctx context.Context, params *ListFilters, cursor *Cursor, limit int) ([]Site, *Cursor, error) {
	sql, args := buildListQuery(params, cursor, limit)
	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	sites := make([]Site, 0, limit)
	for range limit {
		if !rows.Next() {
			break
		}
		var s Site
		if err := rows.Scan(&s.Domain, &s.Unicode, &s.Address, &s.Status, &s.InStorage, &s.SpamContent, &s.CheckedAt); err != nil {
			return nil, nil, err
		}
		sites = append(sites, s)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	var nextCursor *Cursor
	if len(sites) == limit {
		last := sites[len(sites)-1]
		var val any
		switch params.SortBy {
		case SortByCheckedAt:
			val = last.CheckedAt.Unix()
		default:
		}
		nextCursor = &Cursor{
			Value:  val,
			Domain: last.Domain,
		}
	}
	return sites, nextCursor, nil
}

func (r *SitesStore) ReserveCheck(ctx context.Context, stale time.Duration, hold time.Duration, limit int) ([]string, error) {
	const sql = `
	update sites
	set checking_until = now() + $2
	from (
		select domain from sites
		where checked_at + $1 < now()
			and (checking_until is null or checking_until < now())
		order by checked_at asc
		limit $3
		for update skip locked
	) as stale
	where sites.domain = stale.domain
	returning sites.domain
	`
	rows, err := r.db.Query(ctx, sql, stale, hold, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	res := make([]string, 0, limit)
	for rows.Next() {
		var domain string
		if err := rows.Scan(&domain); err != nil {
			return nil, err
		}
		res = append(res, domain)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func (r *SitesStore) FinalizeCheck(ctx context.Context, domain string, status SiteStatus, inStorage bool, spamContent bool) error {
	const sql = `
	update sites set
		status = $2,
		in_storage = $3,
		spam_content = $4,
		checked_at = now(),
		checking_until = null
	where domain = $1
	`
	_, err := r.db.Exec(ctx, sql, domain, status, inStorage, spamContent)
	return err
}

func (r *SitesStore) IsBanned(ctx context.Context, domain string) (bool, error) {
	const sql = `
	select exists(
		select 1 from sites
		where domain = $1
	)
	`
	var exists bool
	err := r.db.QueryRow(ctx, sql, domain).Scan(&exists)
	return exists, err
}

func (r *SitesStore) AddDomains(ctx context.Context, sites ...SiteCreate) error {
	const sql = `
	insert into sites (domain, unicode, zone, address)
	select * from unnest($1::text[], $2::text[], $3::text[], $4::text[])
	on conflict (domain) do nothing
	`
	domains := make([]string, len(sites))
	unicodes := make([]string, len(sites))
	zones := make([]string, len(sites))
	addresses := make([]string, len(sites))

	for i, site := range sites {
		domains[i] = site.Domain
		unicodes[i] = site.Unicode
		zones[i] = site.Zone
		addresses[i] = site.Address
	}
	_, err := r.db.Exec(ctx, sql, domains, unicodes, zones, addresses)
	return err
}

func buildListQuery(params *ListFilters, cursor *Cursor, limit int) (string, []any) {
	const baseSql = `
	select domain, unicode, address, status, in_storage, spam_content, checked_at from sites
	%s
	order by %s
	limit $%d
	`
	wheres := make([]string, 0)
	args := make([]any, 0)

	if params.Search != "" {
		wheres = append(wheres, fmt.Sprintf("domain ilike $%d", len(args)+1))
		args = append(args, "%"+escapeLikeSearch(params.Search)+"%")
	}

	if params.Inaccessible {
		wheres = append(wheres, fmt.Sprintf("status != %d", StatusNoSite))
	} else {
		wheres = append(wheres, fmt.Sprintf("status = %d", StatusAccessible))
	}
	if params.Punycode != nil {
		match := "="
		if *params.Punycode {
			match = "!="
		}
		wheres = append(wheres, fmt.Sprintf("domain %s unicode", match))
	}
	if !params.Spam {
		wheres = append(wheres, "spam_content = false")
	}
	if params.Zone != "" {
		wheres = append(wheres, fmt.Sprintf("zone = $%d", len(args)+1))
		args = append(args, params.Zone)
	}

	if cursor != nil {
		if params.SortBy == SortByDomain {
			comp := ">"
			if params.Desc {
				comp = "<"
			}
			wheres = append(wheres, fmt.Sprintf("domain %s $%d", comp, len(args)+1))
			args = append(args, cursor.Domain)
		} else {
			comp := ">"
			if params.Desc {
				comp = "<"
			}
			wheres = append(wheres, fmt.Sprintf(
				"%s %s $%d or (%s = $%d and domain > $%d)",
				params.SortBy, comp, len(args)+1, params.SortBy, len(args)+1, len(args)+2,
			))
			args = append(args, cursor.Value, cursor.Domain)
		}
	}

	var orderClause string
	order := "asc"
	if params.Desc {
		order = "desc"
	}
	if params.SortBy == SortByDomain {
		orderClause = fmt.Sprintf("domain %s", order)
	} else {
		orderClause = fmt.Sprintf("%s %s, domain asc", params.SortBy, order)
	}
	whereClause := ""
	if len(wheres) > 0 {
		whereClause = "where " + strings.Join(wheres, " and ")
	}
	sql := fmt.Sprintf(baseSql, whereClause, orderClause, len(args)+1)
	args = append(args, limit)
	return sql, args
}
