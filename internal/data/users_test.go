package data

import (
	"crypto/sha256"
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

	"cinemesis/internal/validator"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

func TestUser_IsAnonymous(t *testing.T) {
	t.Run("Anonymous user", func(t *testing.T) {
		user := AnonymousUser
		assert.True(t, user.IsAnonymous())
	})

	t.Run("Non-anonymous user", func(t *testing.T) {
		user := &User{ID: 1, Name: "Test User"}
		assert.False(t, user.IsAnonymous())
	})
}

func TestPassword_Set(t *testing.T) {
	t.Run("Valid password", func(t *testing.T) {
		p := &password{}
		err := p.Set("password123")
		assert.NoError(t, err)
		assert.NotNil(t, p.plaintext)
		assert.Equal(t, "password123", *p.plaintext)
		assert.NotNil(t, p.hash)
		assert.True(t, len(p.hash) > 0)
	})

	t.Run("Invalid password", func(t *testing.T) {
		p := &password{}
		err := p.Set("validpassword")
		assert.NoError(t, err)
	})
}


func TestPassword_Matches(t *testing.T) {
	t.Run("Matching password", func(t *testing.T) {
		p := &password{}
		err := p.Set("password123")
		require.NoError(t, err)

		matches, err := p.Matches("password123")
		assert.NoError(t, err)
		assert.True(t, matches)
	})

	t.Run("Non-matching password", func(t *testing.T) {
		p := &password{}
		err := p.Set("password123")
		require.NoError(t, err)

		matches, err := p.Matches("wrongpassword")
		assert.NoError(t, err)
		assert.False(t, matches)
	})

	t.Run("Invalid hash", func(t *testing.T) {
		p := &password{hash: []byte("invalidhash")}
		matches, err := p.Matches("password123")
		assert.Error(t, err)
		assert.False(t, matches)
	})
}

func TestValidateEmail(t *testing.T) {
	tests := []struct {
		name    string
		email   string
		wantErr bool
		errors  map[string]string
	}{
		{
			name:    "Valid email",
			email:   "test@example.com",
			wantErr: false,
		},
		{
			name:    "Empty email",
			email:   "",
			wantErr: true,
			errors:  map[string]string{"email": "must be provided"},
		},
		{
			name:    "Invalid email",
			email:   "invalid-email",
			wantErr: true,
			errors:  map[string]string{"email": "must be a valid email address"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := validator.New()
			ValidateEmail(v, tt.email)
			if tt.wantErr {
				assert.False(t, v.Valid())
				assert.Equal(t, tt.errors, v.Errors)
			} else {
				assert.True(t, v.Valid())
			}
		})
	}
}

func TestValidatePasswordPlaintext(t *testing.T) {
	tests := []struct {
		name    string
		pass    string
		wantErr bool
		errors  map[string]string
	}{
		{
			name:    "Valid password",
			pass:    "password123",
			wantErr: false,
		},
		{
			name:    "Empty password",
			pass:    "",
			wantErr: true,
			errors:  map[string]string{"password": "must be provided"},
		},
		{
			name:    "Too short password",
			pass:    "pass",
			wantErr: true,
			errors:  map[string]string{"password": "must be at least 8 bytes long"},
		},
		{
			name:    "Too long password",
			pass:    string(make([]byte, 73)),
			wantErr: true,
			errors:  map[string]string{"password": "must not be more than 72 bytes long"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := validator.New()
			ValidatePasswordPlaintext(v, tt.pass)
			if tt.wantErr {
				assert.False(t, v.Valid())
				assert.Equal(t, tt.errors, v.Errors)
			} else {
				assert.True(t, v.Valid())
			}
		})
	}
}

func TestValidateUser(t *testing.T) {
	tests := []struct {
		name    string
		user    *User
		wantErr bool
		errors  map[string]string
	}{
		{
			name: "Valid user",
			user: &User{
				Name:     "Test User",
				Email:    "test@example.com",
				Password: password{hash: []byte("validhash")},
			},
			wantErr: false,
		},
		{
			name: "Missing name",
			user: &User{
				Name:     "",
				Email:    "test@example.com",
				Password: password{hash: []byte("validhash")},
			},
			wantErr: true,
			errors:  map[string]string{"name": "must be provided"},
		},
		{
			name: "Name too long",
			user: &User{
				Name:     string(make([]byte, 501)),
				Email:    "test@example.com",
				Password: password{hash: []byte("validhash")},
			},
			wantErr: true,
			errors:  map[string]string{"name": "must not be more than 500 bytes long"},
		},
		{
			name: "Invalid email",
			user: &User{
				Name:     "Test User",
				Email:    "invalid",
				Password: password{hash: []byte("validhash")},
			},
			wantErr: true,
			errors:  map[string]string{"email": "must be a valid email address"},
		},
		{
			name: "Invalid password",
			user: &User{
				Name:     "Test User",
				Email:    "test@example.com",
				Password: password{plaintext: ptr("short"), hash: []byte("validhash")},
			},
			wantErr: true,
			errors:  map[string]string{"password": "must be at least 8 bytes long"},
		},
		{
			name: "Missing password hash",
			user: &User{
				Name:     "Test User",
				Email:    "test@example.com",
				Password: password{plaintext: ptr("password123")},
			},
			wantErr: true,
			// Should panic, so we expect a panic
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := validator.New()
			if tt.name == "Missing password hash" {
				assert.Panics(t, func() { ValidateUser(v, tt.user) }, "expected panic for missing password hash")
				return
			}
			ValidateUser(v, tt.user)
			if tt.wantErr {
				assert.False(t, v.Valid())
				assert.Equal(t, tt.errors, v.Errors)
			} else {
				assert.True(t, v.Valid())
			}
		})
	}
}

func TestUserModel_Insert(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	m := UserModel{DB: db}
	user := &User{
		Name:      "Test User",
		Email:     "test@example.com",
		Password:  password{},
		Activated: false,
	}
	err = user.Password.Set("password123")
	require.NoError(t, err)

	fixedCreatedAt := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)

	t.Run("Success", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`
        INSERT INTO users (name, email, password_hash, activated) 
        VALUES ($1, $2, $3, $4)
        RETURNING id, created_at, version`)).
			WithArgs(user.Name, user.Email, user.Password.hash, user.Activated).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "version"}).
				AddRow(1, fixedCreatedAt, 1))

		err := m.Insert(user)
		assert.NoError(t, err)
		assert.Equal(t, int64(1), user.ID)
		assert.Equal(t, fixedCreatedAt, user.CreatedAt)
		assert.Equal(t, 1, user.Version)

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Duplicate email", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`
        INSERT INTO users (name, email, password_hash, activated) 
        VALUES ($1, $2, $3, $4)
        RETURNING id, created_at, version`)).
			WithArgs(user.Name, user.Email, user.Password.hash, user.Activated).
			WillReturnError(errors.New("pq: duplicate key value violates unique constraint \"users_email_key\""))

		err := m.Insert(user)
		assert.ErrorIs(t, err, ErrDuplicateEmail)

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestUserModel_GetByEmail(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	m := UserModel{DB: db}
	fixedCreatedAt := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	passwordHash, err := bcrypt.GenerateFromPassword([]byte("password123"), 12)
	require.NoError(t, err)

	t.Run("Success", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`
        SELECT id, created_at, name, email, password_hash, activated, version
        FROM users
        WHERE email = $1`)).
			WithArgs("test@example.com").
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "name", "email", "password_hash", "activated", "version"}).
				AddRow(1, fixedCreatedAt, "Test User", "test@example.com", passwordHash, true, 1))

		user, err := m.GetByEmail("test@example.com")
		assert.NoError(t, err)
		assert.Equal(t, int64(1), user.ID)
		assert.Equal(t, "Test User", user.Name)
		assert.Equal(t, "test@example.com", user.Email)
		assert.True(t, user.Activated)
		assert.Equal(t, 1, user.Version)

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Not found", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`
        SELECT id, created_at, name, email, password_hash, activated, version
        FROM users
        WHERE email = $1`)).
			WithArgs("notfound@example.com").
			WillReturnError(sql.ErrNoRows)

		_, err := m.GetByEmail("notfound@example.com")
		assert.ErrorIs(t, err, ErrRecordNotFound)

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestUserModel_Update(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	m := UserModel{DB: db}
	user := &User{
		ID:        1,
		Name:      "Updated User",
		Email:     "updated@example.com",
		Password:  password{},
		Activated: true,
		Version:   1,
	}
	err = user.Password.Set("newpassword")
	require.NoError(t, err)

	t.Run("Success", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`
        UPDATE users 
        SET name = $1, email = $2, password_hash = $3, activated = $4, version = version + 1
        WHERE id = $5 AND version = $6
        RETURNING version`)).
			WithArgs(user.Name, user.Email, user.Password.hash, user.Activated, user.ID, user.Version).
			WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(2))

		err := m.Update(user)
		assert.NoError(t, err)
		assert.Equal(t, 2, user.Version)

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Duplicate email", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`
        UPDATE users 
        SET name = $1, email = $2, password_hash = $3, activated = $4, version = version + 1
        WHERE id = $5 AND version = $6
        RETURNING version`)).
			WithArgs(user.Name, user.Email, user.Password.hash, user.Activated, user.ID, user.Version).
			WillReturnError(errors.New("pq: duplicate key value violates unique constraint \"users_email_key\""))

		err := m.Update(user)
		assert.ErrorIs(t, err, ErrDuplicateEmail)

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Edit conflict", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`
        UPDATE users 
        SET name = $1, email = $2, password_hash = $3, activated = $4, version = version + 1
        WHERE id = $5 AND version = $6
        RETURNING version`)).
			WithArgs(user.Name, user.Email, user.Password.hash, user.Activated, user.ID, user.Version).
			WillReturnError(sql.ErrNoRows)

		err := m.Update(user)
		assert.ErrorIs(t, err, ErrEditConflict)

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestUserModel_GetForToken(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	m := UserModel{DB: db}
	fixedCreatedAt := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	passwordHash, err := bcrypt.GenerateFromPassword([]byte("password123"), 12)
	require.NoError(t, err)
	tokenPlaintext := "validtoken123"
	tokenHash := sha256.Sum256([]byte(tokenPlaintext))
	tokenScope := "activation"

	t.Run("Success", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`
        SELECT users.id, users.created_at, users.name, users.email, users.password_hash, users.activated, users.version
        FROM users
        INNER JOIN tokens
        ON users.id = tokens.user_id
        WHERE tokens.hash = $1
        AND tokens.scope = $2 
        AND tokens.expiry > $3`)).
			WithArgs(tokenHash[:], tokenScope, sqlmock.AnyArg()).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "name", "email", "password_hash", "activated", "version"}).
				AddRow(1, fixedCreatedAt, "Test User", "test@example.com", passwordHash, true, 1))

		user, err := m.GetForToken(tokenScope, tokenPlaintext)
		assert.NoError(t, err)
		assert.Equal(t, int64(1), user.ID)
		assert.Equal(t, "Test User", user.Name)
		assert.Equal(t, "test@example.com", user.Email)
		assert.True(t, user.Activated)
		assert.Equal(t, 1, user.Version)

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Not found", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`
        SELECT users.id, users.created_at, users.name, users.email, users.password_hash, users.activated, users.version
        FROM users
        INNER JOIN tokens
        ON users.id = tokens.user_id
        WHERE tokens.hash = $1
        AND tokens.scope = $2 
        AND tokens.expiry > $3`)).
			WithArgs(tokenHash[:], tokenScope, sqlmock.AnyArg()).
			WillReturnError(sql.ErrNoRows)

		_, err := m.GetForToken(tokenScope, tokenPlaintext)
		assert.ErrorIs(t, err, ErrRecordNotFound)

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

// Helper function to create a pointer to a string
func ptr(s string) *string {
	return &s
}
