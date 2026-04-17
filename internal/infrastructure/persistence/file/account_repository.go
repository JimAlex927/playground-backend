package file

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"playground/internal/domain/account"
)

type AccountRepository struct {
	path string
	mu   sync.Mutex
}

type snapshot struct {
	Accounts []account.Account `json:"accounts"`
}

func NewAccountRepository(path string) *AccountRepository {
	return &AccountRepository{path: path}
}

func (r *AccountRepository) List(context.Context) ([]account.Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	state, err := r.load()
	if err != nil {
		return nil, err
	}

	items := append([]account.Account(nil), state.Accounts...)
	slices.SortFunc(items, func(a, b account.Account) int {
		return strings.Compare(a.Username, b.Username)
	})
	return items, nil
}

func (r *AccountRepository) GetByID(_ context.Context, id string) (account.Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	state, err := r.load()
	if err != nil {
		return account.Account{}, err
	}

	for _, item := range state.Accounts {
		if item.ID == id {
			return item, nil
		}
	}

	return account.Account{}, account.ErrNotFound
}

func (r *AccountRepository) FindByLoginID(_ context.Context, loginID string) (account.Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	state, err := r.load()
	if err != nil {
		return account.Account{}, err
	}

	normalized := account.NormalizeLoginID(loginID)
	for _, item := range state.Accounts {
		if account.NormalizeLoginID(item.Username) == normalized || account.NormalizeLoginID(item.Email) == normalized {
			return item, nil
		}
	}

	return account.Account{}, account.ErrNotFound
}

func (r *AccountRepository) Create(_ context.Context, item account.Account) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	state, err := r.load()
	if err != nil {
		return err
	}

	if err := ensureUnique(state.Accounts, item, ""); err != nil {
		return err
	}

	state.Accounts = append(state.Accounts, item)
	return r.save(state)
}

func (r *AccountRepository) Update(_ context.Context, item account.Account) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	state, err := r.load()
	if err != nil {
		return err
	}

	if err := ensureUnique(state.Accounts, item, item.ID); err != nil {
		return err
	}

	for i := range state.Accounts {
		if state.Accounts[i].ID == item.ID {
			state.Accounts[i] = item
			return r.save(state)
		}
	}

	return account.ErrNotFound
}

func (r *AccountRepository) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	state, err := r.load()
	if err != nil {
		return err
	}

	for i := range state.Accounts {
		if state.Accounts[i].ID == id {
			state.Accounts = append(state.Accounts[:i], state.Accounts[i+1:]...)
			return r.save(state)
		}
	}

	return account.ErrNotFound
}

func (r *AccountRepository) load() (snapshot, error) {
	var state snapshot
	data, err := os.ReadFile(r.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return snapshot{}, nil
		}
		return snapshot{}, fmt.Errorf("read account store: %w", err)
	}

	if len(data) == 0 {
		return snapshot{}, nil
	}

	if err := json.Unmarshal(data, &state); err != nil {
		return snapshot{}, fmt.Errorf("decode account store: %w", err)
	}

	return state, nil
}

func (r *AccountRepository) save(state snapshot) error {
	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode account store: %w", err)
	}

	tempFile := r.path + ".tmp"
	if err := os.WriteFile(tempFile, data, 0o644); err != nil {
		return fmt.Errorf("write account store temp file: %w", err)
	}

	if err := os.Remove(r.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove previous account store: %w", err)
	}

	if err := os.Rename(tempFile, r.path); err != nil {
		return fmt.Errorf("replace account store: %w", err)
	}

	return nil
}

func ensureUnique(accounts []account.Account, candidate account.Account, ignoreID string) error {
	candidateUsername := account.NormalizeLoginID(candidate.Username)
	candidateEmail := account.NormalizeLoginID(candidate.Email)

	for _, item := range accounts {
		if item.ID == ignoreID {
			continue
		}

		if account.NormalizeLoginID(item.Username) == candidateUsername {
			return account.ErrDuplicateUsername
		}

		if candidateEmail != "" && account.NormalizeLoginID(item.Email) == candidateEmail {
			return account.ErrDuplicateEmail
		}
	}

	return nil
}
