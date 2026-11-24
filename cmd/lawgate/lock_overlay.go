package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type replayOverlay struct {
	Replace map[string]string
}

func (replay baseReplay) makeOverlay(temp string, files []string) (string, error) {
	overlay := replayOverlay{Replace: make(map[string]string)}
	dirs := make(map[string]bool)
	for index, name := range files {
		if err := replay.restoreTest(temp, name, index, overlay); err != nil {
			return "", err
		}
		dirs[filepath.Dir(name)] = true
	}
	for dir := range dirs {
		if err := overlay.hideCandidateTests(replay.root, dir); err != nil {
			return "", err
		}
	}
	data, err := json.Marshal(overlay)
	if err != nil {
		return "", err
	}
	name := filepath.Join(temp, "overlay.json")
	return name, os.WriteFile(name, data, 0600)
}

func (replay baseReplay) restoreTest(temp, name string, index int, overlay replayOverlay) error {
	if err := refuseLinkedPath(replay.root, filepath.FromSlash(name)); err != nil {
		return err
	}
	data, err := replay.read(name)
	if err != nil {
		return err
	}
	replacement := filepath.Join(temp, fmt.Sprintf("%d_test.go", index))
	overlay.Replace[filepath.Join(replay.root, name)] = replacement
	return os.WriteFile(replacement, data, 0600)
}

// Extra candidate test files could alter the approved test through init or TestMain.
func (overlay replayOverlay) hideCandidateTests(root, dir string) error {
	files, err := os.ReadDir(filepath.Join(root, dir))
	if err != nil {
		return err
	}
	for _, file := range files {
		name := filepath.Join(root, dir, file.Name())
		_, restored := overlay.Replace[name]
		if strings.HasSuffix(file.Name(), "_test.go") && !restored {
			overlay.Replace[name] = ""
		}
	}
	return nil
}
