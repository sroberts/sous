package recipes

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
	"unicode"

	"github.com/sroberts/sous/internal/identity"
)

// ListPeople returns every household member, ordered by name.
func (s *Service) ListPeople(ctx context.Context, c identity.Caller) ([]Person, error) {
	out := []Person{}
	err := s.st.Read(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT username, name FROM people ORDER BY name COLLATE NOCASE, username`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var p Person
			if err := rows.Scan(&p.Username, &p.Name); err != nil {
				return err
			}
			out = append(out, p)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

var slugRE = regexp.MustCompile(`[^a-z0-9-]+`)

// slug is the username derived from a display name. Usernames stay ASCII
// because browsers refuse non-Latin-1 header values, and the UI sends the
// username in X-Sous-Person. A name with letters but none of them ASCII
// ("李明") gets a stable "p-" form instead of being rejected.
func slug(name string) string {
	if s := strings.Trim(slugRE.ReplaceAllString(strings.ToLower(name), "-"), "-"); s != "" {
		return s
	}
	if !strings.ContainsFunc(name, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) {
		return ""
	}
	sum := sha256.Sum256([]byte(name))
	return "p-" + hex.EncodeToString(sum[:4])
}

// CreatePerson adds a local person named name. It needs no caller. The
// username is name lower-cased, each run of [^a-z0-9-] replaced with "-",
// and "-" trimmed from both ends. An empty name or username is CodeInvalid;
// a taken username is CodeExists. It logs a person create change.
func (s *Service) CreatePerson(ctx context.Context, c identity.Caller, name string) (Person, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Person{}, Errorf(CodeInvalid, "name is required")
	}
	p := Person{Username: slug(name), Name: name}
	if p.Username == "" {
		return Person{}, Errorf(CodeInvalid, "name needs at least one letter or digit")
	}
	now := s.stamp()
	err := s.st.Write(ctx, func(tx *sql.Tx) error {
		var one int
		err := tx.QueryRowContext(ctx, `SELECT 1 FROM people WHERE username = ?`, p.Username).Scan(&one)
		if err == nil {
			return Errorf(CodeExists, "%s is already in the household", name)
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO people (username, name, source, created_at, updated_at)
			VALUES (?, ?, 'local', ?, ?)`, p.Username, p.Name, now, now); err != nil {
			return err
		}
		return s.logChange(ctx, tx, c, now, "person", p.Username, "create", "Added person "+p.Name)
	})
	if err != nil {
		return Person{}, err
	}
	return p, nil
}

// Lookup returns the person with username, or (nil, nil) if there is none.
// It implements identity.Directory, so it takes no Caller.
func (s *Service) Lookup(ctx context.Context, username string) (*identity.Person, error) {
	var p *identity.Person
	err := s.st.Read(ctx, func(tx *sql.Tx) error {
		var q identity.Person
		err := tx.QueryRowContext(ctx, `SELECT username, name, coalesce(email, '') FROM people WHERE username = ?`,
			username).Scan(&q.Username, &q.Name, &q.Email)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		} else if err != nil {
			return err
		}
		p = &q
		return nil
	})
	return p, err
}

// UpsertGateway creates or updates p with source "gateway", only writing
// when the stored name or email differs. It implements identity.Directory,
// so it takes no Caller.
func (s *Service) UpsertGateway(ctx context.Context, p identity.Person) error {
	if p.Username == "" {
		return errors.New("recipes: gateway person has no username")
	}
	var email *string
	if p.Email != "" {
		email = &p.Email
	}
	now := s.stamp()
	return s.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO people (username, name, email, source, created_at, updated_at)
			VALUES (?1, ?2, ?3, 'gateway', ?4, ?4)
			ON CONFLICT (username) DO UPDATE SET name = ?2, email = ?3, source = 'gateway', updated_at = ?4
			WHERE name IS NOT ?2 OR email IS NOT ?3 OR source IS NOT 'gateway'`,
			p.Username, p.Name, email, now)
		return err
	})
}
