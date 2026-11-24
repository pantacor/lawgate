package rulechanges

import "errors"

// ReadSnapshot exposes regular Git blobs from one immutable commit without a checkout.
// The reader is valid for the duration of use.
func ReadSnapshot(root, ref string, use func(func(string) ([]byte, error)) error) (err error) {
	batch, err := startBlobs(root)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, batch.close()) }()
	repo := history{root: root, blobs: batch.read, command: func(args ...string) ([]byte, error) {
		return gitCommand(root, args...)
	}}
	commit, err := repo.commitID(ref)
	if err != nil {
		return err
	}
	tree, err := repo.sourceAt(commit)
	if err != nil {
		return err
	}
	return use(tree.read)
}
