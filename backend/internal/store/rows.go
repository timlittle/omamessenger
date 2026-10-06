package store

import "database/sql"

// scanner is a single row from either *sql.Row or *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

// scanAll reads every row with scan and closes rows. It returns an empty,
// non-nil slice when there are no rows, so the UI receives [] not null.
func scanAll[T any](rows *sql.Rows, scan func(scanner) (T, error)) ([]T, error) {
	defer rows.Close()

	out := []T{}
	for rows.Next() {
		item, err := scan(rows)
		if err != nil {
			return nil, err
		}

		out = append(out, item)
	}

	return out, rows.Err()
}
