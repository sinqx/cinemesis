package data

import (
	"database/sql"
	"errors"
)

var (
	ErrRecordNotFound = errors.New("record not found")
	ErrEditConflict   = errors.New("edit conflict")
)

type Models struct {
	Movies      MovieModel
	Reviews     ReviewModel
	Genres      GenreModel
	Tokens      TokenModel
	Users       UserModel
	Permissions PermissionModel
}

func NewModels(db *sql.DB) Models {
	return Models{
		Movies:      MovieModel{DB: db},
		Reviews:     ReviewModel{DB: db},
		Genres:      GenreModel{DB: db},
		Tokens:      TokenModel{DB: db},
		Users:       UserModel{DB: db},
		Permissions: PermissionModel{DB: db},
	}
}
