package config

import (
	"context"
	"fmt"
	"net/url"

	"github.com/redis/go-redis/v9"
)

// ParseRedisURI splits a redis:// URI, the form of the PPANEL_REDIS
// environment variable, into the address (port 6379 when it names none), the
// password and the database number of its path (0 without one). A path that
// is not a number is an error.
func ParseRedisURI(uri string) (addr, password string, database int, err error) {
	parsedURI, err := url.Parse(uri)

	if err != nil {
		return "", "", 0, err
	}
	host := parsedURI.Hostname()
	port := parsedURI.Port()
	if port == "" {
		port = "6379"
	}
	addr = fmt.Sprintf("%s:%s", host, port)

	// password
	if parsedURI.User != nil {
		password, _ = parsedURI.User.Password()
	}
	if len(parsedURI.Path) > 1 { // Path: "/0"
		var dbIndex int
		_, err = fmt.Sscanf(parsedURI.Path, "/%d", &dbIndex)
		if err == nil {
			database = dbIndex
		}
	}
	return
}

// RedisPing reports whether the Redis server at addr answers a PING with the
// given password and database. It opens a client for this one ping and closes
// it afterwards; ctx bounds the whole attempt, the client's dial retries
// included.
func RedisPing(ctx context.Context, addr, password string, database int) error {
	rds := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       database,
	})
	// The client did nothing but ping; a failed close has nothing to report.
	defer func() { _ = rds.Close() }()
	return rds.Ping(ctx).Err()
}
