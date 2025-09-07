package api

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/oxylume/index/internal/db"
)

func GetBool(q url.Values, key string) (v bool, ok bool) {
	val := q.Get(key)
	if val == "" {
		return false, false
	}
	return (val == "1" || val == "true"), true
}

func GetInt(q url.Values, key string) (v int, ok bool, err error) {
	val := q.Get(key)
	if val == "" {
		return 0, false, nil
	}
	v, err = strconv.Atoi(val)
	if err != nil {
		return 0, true, fmt.Errorf("invalid int value %s for key %s", val, key)
	}
	return v, true, nil
}

func EncodeCursor(c *db.Cursor) string {
	var raw string
	if c.Value == nil {
		raw = c.Domain
	} else {
		raw = fmt.Sprintf("%v:%s", c.Value, c.Domain)
	}
	return base64.URLEncoding.EncodeToString([]byte(raw))
}

func DecodeCursor(s string, sortBy db.SortBy) (*db.Cursor, error) {
	data, err := base64.URLEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}
	raw := string(data)
	if sortBy == db.SortByDomain {
		return &db.Cursor{Domain: raw}, nil
	}
	v, domain, ok := strings.Cut(raw, ":")
	if !ok {
		return nil, fmt.Errorf("invalid cursor format %s", raw)
	}
	switch sortBy {
	case db.SortByCheckedAt:
		secs, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid cursor value %s", v)
		}
		val := time.Unix(secs, 0)
		return &db.Cursor{Value: val, Domain: domain}, nil
	default:
		return nil, fmt.Errorf("unsupported sort by %s", sortBy)
	}
}
