package recipes

import (
	"context"
	"database/sql"
	"strconv"
	"strings"

	"github.com/sroberts/sous/internal/identity"
)

// Changes returns the change log, newest first, filtered by q.
func (s *Service) Changes(ctx context.Context, c identity.Caller, q ChangeQuery) (ChangePage, error) {
	limit, err := pageLimit(q.Limit)
	if err != nil {
		return ChangePage{}, err
	}
	var where []string
	var args []any
	if q.EntityID != "" {
		where, args = append(where, "entity_id = ?"), append(args, q.EntityID)
	}
	if q.Person != "" {
		where, args = append(where, "person_id = ?"), append(args, q.Person)
	}
	switch identity.Via(q.Via) {
	case "":
	case identity.ViaUI, identity.ViaAPI, identity.ViaMCP, identity.ViaSystem:
		where, args = append(where, "via = ?"), append(args, q.Via)
	default:
		return ChangePage{}, Errorf(CodeInvalid, "via must be ui, api, mcp or system")
	}
	if q.Cursor != "" {
		cur, err := decodeCursor(q.Cursor, "changes")
		if err != nil {
			return ChangePage{}, err
		}
		seq, err := strconv.ParseInt(cur.Key, 10, 64)
		if err != nil {
			return ChangePage{}, Errorf(CodeInvalidCursor, "cursor is not valid for this list")
		}
		where, args = append(where, "seq < ?"), append(args, seq)
	}
	cond := ""
	if len(where) > 0 {
		cond = "WHERE " + strings.Join(where, " AND ")
	}
	out := ChangePage{Items: []Change{}}
	err = s.st.Read(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT seq, id, at, person_id, via, entity, entity_id, action, summary
			FROM changes `+cond+` ORDER BY seq DESC LIMIT ?`, append(args, limit+1)...)
		if err != nil {
			return err
		}
		defer rows.Close()
		var seqs []int64
		for rows.Next() {
			var ch Change
			var seq int64
			if err := rows.Scan(&seq, &ch.ID, &ch.At, &ch.PersonID, &ch.Via, &ch.Entity, &ch.EntityID, &ch.Action, &ch.Summary); err != nil {
				return err
			}
			out.Items = append(out.Items, ch)
			seqs = append(seqs, seq)
		}
		if len(out.Items) > limit {
			out.Items = out.Items[:limit]
			out.NextCursor = encodeCursor(cursor{Kind: "changes", Key: strconv.FormatInt(seqs[limit-1], 10)})
		}
		return rows.Err()
	})
	if err != nil {
		return ChangePage{}, err
	}
	return out, nil
}
