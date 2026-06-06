package spawn

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
)

var (
	ErrUnauthorized  = errors.New("unauthorized")
	ErrLimitReached  = errors.New("drone limit reached")
	ErrConflict      = errors.New("spawn conflict")
	ErrAlreadyExists = errors.New("drone already exists")
)

type PodStore interface {
	ListDroneIDs(ctx context.Context) ([]int, error)
	CreateDrone(ctx context.Context, id int) error
}

type Service struct {
	store     PodStore
	password  string
	maxDrones int
}

func NewService(store PodStore, password string, maxDrones int) *Service {
	return &Service{
		store:     store,
		password:  password,
		maxDrones: maxDrones,
	}
}

func (s *Service) Spawn(ctx context.Context, supplied string) (int, error) {
	if subtle.ConstantTimeCompare([]byte(s.password), []byte(supplied)) != 1 {
		return 0, ErrUnauthorized
	}

	for attempt := 0; attempt < 3; attempt++ {
		ids, err := s.store.ListDroneIDs(ctx)
		if err != nil {
			return 0, fmt.Errorf("list drones: %w", err)
		}
		if len(ids) >= s.maxDrones {
			return 0, ErrLimitReached
		}

		used := make(map[int]bool, len(ids))
		for _, id := range ids {
			used[id] = true
		}

		for id := 4; id <= s.maxDrones; id++ {
			if used[id] {
				continue
			}
			err = s.store.CreateDrone(ctx, id)
			if errors.Is(err, ErrAlreadyExists) {
				break
			}
			if err != nil {
				return 0, fmt.Errorf("create drone %d: %w", id, err)
			}
			return id, nil
		}
	}

	return 0, ErrConflict
}
