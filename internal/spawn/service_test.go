package spawn

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type fakeStore struct {
	ids             []int
	listErr         error
	createErr       error
	alreadyExists   map[int]bool
	created         []int
	listCallCount   int
	createCallCount int
}

func (s *fakeStore) ListDroneIDs(context.Context) ([]int, error) {
	s.listCallCount++
	if s.listErr != nil {
		return nil, s.listErr
	}
	ids := make([]int, len(s.ids))
	copy(ids, s.ids)
	return ids, nil
}

func (s *fakeStore) CreateDrone(_ context.Context, id int) error {
	s.createCallCount++
	s.created = append(s.created, id)
	if s.alreadyExists[id] {
		s.ids = append(s.ids, id)
		delete(s.alreadyExists, id)
		return ErrAlreadyExists
	}
	if s.createErr != nil {
		return s.createErr
	}
	s.ids = append(s.ids, id)
	return nil
}

func TestSpawnSelectsFirstAvailableID(t *testing.T) {
	store := &fakeStore{ids: []int{1, 2, 3, 4, 6}}
	service := NewService(store, 20)

	id, err := service.Spawn(context.Background())
	if err != nil {
		t.Fatalf("Spawn returned error: %v", err)
	}
	if id != 5 {
		t.Fatalf("Spawn ID = %d, want 5", id)
	}
	if !reflect.DeepEqual(store.created, []int{5}) {
		t.Fatalf("created IDs = %v, want [5]", store.created)
	}
}

func TestSpawnRejectsAtTwentyDrones(t *testing.T) {
	store := &fakeStore{ids: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}}
	service := NewService(store, 20)

	id, err := service.Spawn(context.Background())
	if !errors.Is(err, ErrLimitReached) {
		t.Fatalf("error = %v, want ErrLimitReached", err)
	}
	if id != 0 {
		t.Fatalf("id = %d, want 0", id)
	}
	if store.createCallCount != 0 {
		t.Fatalf("CreateDrone called %d times, want 0", store.createCallCount)
	}
}

func TestSpawnUsesLowIDsWhenBaselineDronesAreRemoved(t *testing.T) {
	store := &fakeStore{ids: []int{4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}}
	service := NewService(store, 20)

	id, err := service.Spawn(context.Background())
	if err != nil {
		t.Fatalf("Spawn returned error: %v", err)
	}
	if id != 1 {
		t.Fatalf("Spawn ID = %d, want 1", id)
	}
	if !reflect.DeepEqual(store.created, []int{1}) {
		t.Fatalf("created IDs = %v, want [1]", store.created)
	}
}

func TestSpawnRetriesAlreadyExists(t *testing.T) {
	store := &fakeStore{
		ids:           []int{1, 2, 3},
		alreadyExists: map[int]bool{4: true},
	}
	service := NewService(store, 20)

	id, err := service.Spawn(context.Background())
	if err != nil {
		t.Fatalf("Spawn returned error: %v", err)
	}
	if id != 5 {
		t.Fatalf("Spawn ID = %d, want 5", id)
	}
	if !reflect.DeepEqual(store.created, []int{4, 5}) {
		t.Fatalf("created IDs = %v, want [4 5]", store.created)
	}
}

func TestSpawnReturnsStoreErrors(t *testing.T) {
	listErr := errors.New("list failed")
	store := &fakeStore{listErr: listErr}
	service := NewService(store, 20)

	id, err := service.Spawn(context.Background())
	if !errors.Is(err, listErr) {
		t.Fatalf("error = %v, want wrapped listErr", err)
	}
	if id != 0 {
		t.Fatalf("id = %d, want 0", id)
	}

	createErr := errors.New("create failed")
	store = &fakeStore{ids: []int{1, 2, 3}, createErr: createErr}
	service = NewService(store, 20)

	id, err = service.Spawn(context.Background())
	if !errors.Is(err, createErr) {
		t.Fatalf("error = %v, want wrapped createErr", err)
	}
	if id != 0 {
		t.Fatalf("id = %d, want 0", id)
	}
}
