package mutate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// overlayFile is the JSON `go build -overlay` reads.
type overlayFile struct {
	Replace map[string]string
}

// WriteOverlay writes the mutant under dir and the overlay that compiles it
// in place of file. It returns the overlay's path.
func WriteOverlay(dir, file string, mutant Mutant) (string, error) {
	if strings.ContainsAny(dir, " \t") {
		return "", errors.New("the mutation directory must not contain spaces: GOFLAGS separates flags by spaces")
	}
	source := filepath.Join(dir, filepath.Base(file))
	if err := os.WriteFile(source, mutant.Source, 0o600); err != nil {
		return "", err
	}
	replace := map[string]string{file: source}
	data, err := json.Marshal(overlayFile{Replace: replace})
	if err != nil {
		return "", err
	}
	overlay := filepath.Join(dir, "overlay.json")
	return overlay, os.WriteFile(overlay, data, 0o600)
}

// Key identifies a mutation run by what decides it: the subject's and the
// proof's fingerprints, and the version of the mutation rules (bumped when
// they change which mutants a subject gets). An unchanged law is not
// mutated again.
func Key(fingerprints ...string) string {
	digest := sha256.New()
	digest.Write([]byte("lawgate mutation v2\n"))
	for _, fingerprint := range fingerprints {
		digest.Write([]byte(fingerprint + "\n"))
	}
	return hex.EncodeToString(digest.Sum(nil))
}

// Cache remembers the runs whose every mutant was killed. A survivor is never
// cached: it is checked again until the code or the proof changes.
type Cache struct {
	Dir string // empty disables the cache
}

// UserCache keeps runs under the user's cache directory, when there is one.
func UserCache() Cache {
	base, err := os.UserCacheDir()
	if err != nil {
		return Cache{}
	}
	return Cache{Dir: filepath.Join(base, "lawgate", "mutation")}
}

// Killed reports whether a run with this key killed every mutant before.
func (cache Cache) Killed(key string) bool {
	if cache.Dir == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(cache.Dir, key))
	return err == nil
}

// RecordKilled remembers a run that killed every mutant.
func (cache Cache) RecordKilled(key string) error {
	if cache.Dir == "" {
		return nil
	}
	if err := os.MkdirAll(cache.Dir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(cache.Dir, key), []byte("killed\n"), 0o600)
}
