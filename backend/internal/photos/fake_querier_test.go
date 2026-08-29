package photos

import (
	"context"
	"sort"
	"sync"

	repo "github.com/IkBenJur/italy-trip/internal/postgres/sqlc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// fakeQuerier is an in-memory repo.Querier. It enforces the one constraint the
// upload path actually leans on — UNIQUE (event_id, client_id) — so the race
// handling can be tested without a database.
type fakeQuerier struct {
	mu     sync.Mutex
	event  repo.Event
	users  map[string]repo.User
	photos []repo.Photo

	// beforeCreate runs inside CreatePhoto, letting a test slip a competing row
	// in to force the unique violation.
	beforeCreate func()
}

var _ repo.Querier = (*fakeQuerier)(nil)

func newFakeQuerier(event repo.Event) *fakeQuerier {
	return &fakeQuerier{event: event, users: map[string]repo.User{}}
}

func (f *fakeQuerier) addUser(user repo.User) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.users[keyOf(user.ID)] = user
}

func (f *fakeQuerier) photoCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.photos)
}

func keyOf(id pgtype.UUID) string { return string(id.Bytes[:]) }

func (f *fakeQuerier) GetCurrentEvent(ctx context.Context) (repo.Event, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.event.ID.Valid {
		return repo.Event{}, pgx.ErrNoRows
	}
	return f.event, nil
}

func (f *fakeQuerier) CountPhotos(ctx context.Context, eventID pgtype.UUID) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var n int64
	for _, photo := range f.photos {
		if photo.EventID == eventID {
			n++
		}
	}
	return n, nil
}

func (f *fakeQuerier) CreatePhoto(ctx context.Context, arg repo.CreatePhotoParams) (repo.Photo, error) {
	if f.beforeCreate != nil {
		f.beforeCreate()
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	for _, photo := range f.photos {
		if photo.EventID == arg.EventID && photo.ClientID == arg.ClientID {
			return repo.Photo{}, &pgconn.PgError{
				Code:    uniqueViolation,
				Message: `duplicate key value violates unique constraint "photos_event_id_client_id_key"`,
			}
		}
	}

	photo := repo.Photo{
		ID:          arg.ID,
		EventID:     arg.EventID,
		UploadedBy:  arg.UploadedBy,
		ClientID:    arg.ClientID,
		StorageKey:  arg.StorageKey,
		ThumbKey:    arg.ThumbKey,
		ContentType: arg.ContentType,
		Width:       arg.Width,
		Height:      arg.Height,
		SizeBytes:   arg.SizeBytes,
		TakenAt:     arg.TakenAt,
	}
	f.photos = append(f.photos, photo)
	return photo, nil
}

func (f *fakeQuerier) FindPhotoByClientId(ctx context.Context, arg repo.FindPhotoByClientIdParams) (repo.Photo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, photo := range f.photos {
		if photo.EventID == arg.EventID && photo.ClientID == arg.ClientID {
			return photo, nil
		}
	}
	return repo.Photo{}, pgx.ErrNoRows
}

func (f *fakeQuerier) FindPhotoById(ctx context.Context, id pgtype.UUID) (repo.Photo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, photo := range f.photos {
		if photo.ID == id {
			return photo, nil
		}
	}
	return repo.Photo{}, pgx.ErrNoRows
}

func (f *fakeQuerier) ListPhotosByEvent(ctx context.Context, eventID pgtype.UUID) ([]repo.Photo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	out := []repo.Photo{}
	for _, photo := range f.photos {
		if photo.EventID == eventID {
			out = append(out, photo)
		}
	}
	// Mirrors "ORDER BY taken_at ASC, id ASC".
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].TakenAt.Time.Equal(out[j].TakenAt.Time) {
			return keyOf(out[i].ID) < keyOf(out[j].ID)
		}
		return out[i].TakenAt.Time.Before(out[j].TakenAt.Time)
	})
	return out, nil
}

func (f *fakeQuerier) FindUserById(ctx context.Context, id pgtype.UUID) (repo.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if user, ok := f.users[keyOf(id)]; ok {
		return user, nil
	}
	return repo.User{}, pgx.ErrNoRows
}

func (f *fakeQuerier) FindUserByEmail(ctx context.Context, email string) (repo.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, user := range f.users {
		if user.Email == email {
			return user, nil
		}
	}
	return repo.User{}, pgx.ErrNoRows
}

func (f *fakeQuerier) CreateUser(ctx context.Context, arg repo.CreateUserParams) (repo.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	user := repo.User{Email: arg.Email, PasswordHash: arg.PasswordHash}
	f.users[arg.Email] = user
	return user, nil
}

func (f *fakeQuerier) ListUsers(ctx context.Context) ([]repo.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]repo.User, 0, len(f.users))
	for _, user := range f.users {
		out = append(out, user)
	}
	return out, nil
}

func (f *fakeQuerier) UpsertSingletonEvent(ctx context.Context, arg repo.UpsertSingletonEventParams) (repo.Event, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.event.Name = arg.Name
	f.event.StartsAt = arg.StartsAt
	f.event.EndsAt = arg.EndsAt
	return f.event, nil
}
