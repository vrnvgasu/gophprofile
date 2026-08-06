package postgres

import (
	"database/sql"
	"errors"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/vrnvgasu/gophprofile/internal/repository"
)

type retriability int

const (
	retriable retriability = iota
	nonRetriable
)

type postgresErrorClassifier struct{}

func newPostgresErrorClassifier() *postgresErrorClassifier {
	return &postgresErrorClassifier{}
}

func (c *postgresErrorClassifier) classify(err error) retriability {
	// Отсутствие записи — нормальный результат запроса, повторять его бессмысленно.
	if errors.Is(err, repository.ErrNotFound) || errors.Is(err, sql.ErrNoRows) {
		return nonRetriable
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		// Ошибки уровня сети до ответа сервера тоже имеет смысл повторить.
		return retriable
	}

	if pgerrcode.IsConnectionException(pgErr.Code) {
		return retriable
	}

	return nonRetriable
}
