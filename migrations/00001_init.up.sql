create table sites (
    domain text primary key,
    unicode text not null,
    zone text not null,
    address text not null,
    status int not null default 0,
    in_storage boolean not null default false,
    spam_content boolean not null default false,
    checked_at timestamptz default 'epoch',
    checking_until timestamptz default null,
    created_at timestamptz default now()
);

create index idx_sites_status on sites(status);
create index idx_sites_sort_checked_at on sites(checked_at, domain);

create table crawler_state (
    dns text primary key,
    last_offset bigint not null default 0
);
