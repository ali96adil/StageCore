package vault

import (
	"hash"
	"io"
	"os"
)

const stagingChunkSize = 4 << 20

func (v *Vault) streamToStaging(staged *os.File, hasher hash.Hash, source io.Reader) (int64, error) {
	buffer := make([]byte, stagingChunkSize)
	writer := io.MultiWriter(staged, hasher)
	var total int64
	for {
		n, readErr := source.Read(buffer)
		if n > 0 {
			if err := v.capacity.Admit(v.stagingRoot, uint64(n)); err != nil {
				return total, err
			}
			written, writeErr := writer.Write(buffer[:n])
			if writeErr != nil {
				return total, writeErr
			}
			if written != n {
				return total, io.ErrShortWrite
			}
			total += int64(written)
		}
		if readErr == io.EOF {
			return total, nil
		}
		if readErr != nil {
			return total, readErr
		}
	}
}
