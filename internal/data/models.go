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
	Genres      GenreModel
	Tokens      TokenModel
	Users       UserModel
	Permissions PermissionModel
}

func NewModels(db *sql.DB) Models {
	genreModel := GenreModel{DB: db}

	return Models{
		Movies:      MovieModel{DB: db, GenreRepository: genreModel},
		Genres:      genreModel,
		Tokens:      TokenModel{DB: db},
		Users:       UserModel{DB: db},
		Permissions: PermissionModel{DB: db},
	}
}
