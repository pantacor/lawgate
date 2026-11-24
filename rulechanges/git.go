package rulechanges

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

func gitProcess(root string, args ...string) *exec.Cmd {
	options := []string{"--no-replace-objects", "--no-optional-locks", "-C", root}
	command := exec.Command("git", append(options, args...)...)
	command.Env = append(os.Environ(), "GIT_NO_LAZY_FETCH=1")
	return command
}

func gitCommand(root string, args ...string) ([]byte, error) {
	command := gitProcess(root, args...)
	output, err := command.Output()
	if err == nil {
		return output, nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return nil, fmt.Errorf("git %s: %w (%s)", args[0], err, bytes.TrimSpace(exit.Stderr))
	}
	return nil, fmt.Errorf("git %s: %w", args[0], err)
}

func splitNUL(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	return strings.Split(strings.TrimSuffix(string(data), "\x00"), "\x00")
}

type blobReader struct {
	process *exec.Cmd
	input   io.WriteCloser
	output  *bufio.Reader
	cache   map[string][]byte
}

func startBlobs(root string) (*blobReader, error) {
	command := gitProcess(root, "cat-file", "--batch")
	input, err := command.StdinPipe()
	if err != nil {
		return nil, err
	}
	output, err := command.StdoutPipe()
	if err != nil {
		input.Close()
		return nil, err
	}
	if err := command.Start(); err != nil {
		input.Close()
		output.Close()
		return nil, err
	}
	return &blobReader{command, input, bufio.NewReader(output), make(map[string][]byte)}, nil
}

func (batch *blobReader) read(id string) ([]byte, error) {
	if data, ok := batch.cache[id]; ok {
		return data, nil
	}
	if !objectID.MatchString(id) {
		return nil, fmt.Errorf("invalid blob ID %q", id)
	}
	if _, err := fmt.Fprintln(batch.input, id); err != nil {
		return nil, err
	}
	size, err := batch.readHeader(id)
	if err != nil {
		return nil, err
	}
	data, err := batch.readBody(id, size)
	if err != nil {
		return nil, err
	}
	batch.cache[id] = data
	return data, nil
}

// readHeader reads cat-file's "<id> blob <size>" line and returns the size.
func (batch *blobReader) readHeader(id string) (int, error) {
	header, err := batch.output.ReadString('\n')
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(header)
	if len(fields) != 3 || fields[1] != "blob" {
		return 0, fmt.Errorf("missing Git blob %s", id)
	}
	size, err := strconv.Atoi(fields[2])
	if err != nil || size < 0 {
		return 0, fmt.Errorf("invalid Git blob size %q", fields[2])
	}
	return size, nil
}

// readBody reads size bytes of content and the newline cat-file ends it
// with.
func (batch *blobReader) readBody(id string, size int) ([]byte, error) {
	data := make([]byte, size)
	if _, err := io.ReadFull(batch.output, data); err != nil {
		return nil, err
	}
	if delimiter, err := batch.output.ReadByte(); err != nil || delimiter != '\n' {
		return nil, fmt.Errorf("unterminated Git blob %s", id)
	}
	return data, nil
}

func (batch *blobReader) close() error {
	return errors.Join(batch.input.Close(), batch.process.Wait())
}
