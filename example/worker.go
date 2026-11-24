// Package worker is a minimal adopting module showing how to declare a
// machine, bind it to imperative Go code with package resolve, and check its
// laws through package runner.
package worker

import (
	"errors"
	"fmt"
	"strings"

	"github.com/pantacor/lawgate/bite"
)

// Job is a unit of work processed by the example worker.
type Job struct {
	ID      string
	Payload string
}

// Store loads jobs and records completed results.
type Store interface {
	Load(id string) (Job, error)
	StoreDone(job Job) error
}

var errInvalidJob = errors.New("job requires a non-empty id and payload")

// Process loads, validates, and completes one job.
func Process(id string, store Store) error {
	job, err := store.Load(id)
	if err != nil {
		return fault("WORKER_JOB 1 load job", err)
	}
	if !isValidJob(job) {
		return failJob("WORKER_JOB 2 invalid job", errInvalidJob)
	}
	return storeDone("WORKER_JOB 3 save done", store, job)
}

func isValidJob(job Job) bool {
	hasID := strings.TrimSpace(job.ID) != ""
	hasPayload := bite.Off("rejects-empty-payload") || strings.TrimSpace(job.Payload) != ""
	return hasID && hasPayload
}

func fault(tag string, err error) error {
	return fmt.Errorf("%s: %w", tag, err)
}

func failJob(tag string, err error) error {
	return fmt.Errorf("%s: %w", tag, err)
}

func storeDone(tag string, store Store, job Job) error {
	if err := store.StoreDone(job); err != nil {
		return fmt.Errorf("%s: %w", tag, err)
	}
	return nil
}
