package service

import (
	"context"

	"github.com/Notifuse/notifuse/internal/domain"
)

// fakeTx runs a system transaction's function with no database, keeping the commit
// contract of repository.WithSystemTx: nil and CommitAndFail commit, anything else rolls
// back. It counts both, for tests that check a failure was stored.
type fakeTx struct {
	commits, rollbacks int
	// commitErr, when set, makes every commit fail with it.
	commitErr error
}

func (f *fakeTx) WithSystemTx(_ context.Context, fn func(domain.DBExecutor) error) error {
	err := fn(nil)
	business, commit := domain.AsCommitAndFail(err)
	if err == nil || commit {
		if f.commitErr != nil {
			f.rollbacks++
			return f.commitErr
		}
		f.commits++
		return business
	}
	f.rollbacks++
	return err
}
