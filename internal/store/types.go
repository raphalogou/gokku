package store

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
	"modernc.org/sqlite"
)

// Errors returned by repositories. Callers map them to API error codes.
var (
	ErrNotFound  = errors.New("not found")
	ErrConflict  = errors.New("already exists")
	ErrLastAdmin = errors.New("at least one active admin is required")
)

// NewID returns a new ULID, the id format of every Gokku table.
func NewID() string { return ulid.Make().String() }

// timeLayout matches strftime('%Y-%m-%dT%H:%M:%fZ', 'now') used by column defaults, so
// stored times sort lexicographically.
const timeLayout = "2006-01-02T15:04:05.000Z"

// Time is a UTC timestamp stored as RFC 3339 text with milliseconds.
type Time struct{ time.Time }

// Now returns the current time truncated to the stored precision.
func Now() Time { return At(time.Now()) }

// At converts t to a stored timestamp.
func At(t time.Time) Time { return Time{t.UTC().Truncate(time.Millisecond)} }

// Value implements driver.Valuer.
func (t Time) Value() (driver.Value, error) { return t.UTC().Format(timeLayout), nil }

// Scan implements sql.Scanner.
func (t *Time) Scan(src any) error {
	s, ok := src.(string)
	if !ok {
		return fmt.Errorf("store.Time: cannot scan %T", src)
	}
	parsed, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return fmt.Errorf("store.Time: %w", err)
	}
	t.Time = parsed.UTC()
	return nil
}

// Ptr returns a pointer to t's time.Time, for optional API fields.
func (t *Time) Ptr() *time.Time {
	if t == nil {
		return nil
	}
	v := t.Time
	return &v
}

// sqliteConstraint is the primary result code SQLITE_CONSTRAINT; extended codes keep it in the low byte.
const sqliteConstraint = 19

// mapError turns SQLite constraint failures into repository errors.
func mapError(err error) error {
	var se *sqlite.Error
	if !errors.As(err, &se) || se.Code()&0xff != sqliteConstraint {
		return err
	}
	switch msg := se.Error(); {
	case strings.Contains(msg, ErrLastAdmin.Error()): // raised by the users_keep_admin_* triggers
		return ErrLastAdmin
	case strings.Contains(msg, "UNIQUE"):
		return fmt.Errorf("%w: %w", ErrConflict, err)
	default:
		return err
	}
}
