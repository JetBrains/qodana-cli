package hash

import (
	"crypto/sha256"
	"crypto/sha512"
	"errors"
	"io"
	"os"
)

// GetSha256 computes a SHA-256 hash sum for a byte stream.
func GetSha256(stream io.Reader) (result [32]byte, err error) {
	hasher := sha256.New()
	_, err = io.Copy(hasher, stream)
	if err != nil {
		return result, err
	}

	copy(result[:], hasher.Sum(nil))
	return result, nil
}

// GetFileSha256 computes a SHA-256 hash sum from an existing file.
func GetFileSha256(path string) (result [32]byte, err error) {
	reader, err := os.Open(path)
	if err != nil {
		return result, err
	}
	defer func() {
		err = errors.Join(err, reader.Close())
	}()

	return GetSha256(reader)
}

// GetSha512 computes a SHA-512 hash sum for a byte stream.
func GetSha512(stream io.Reader) (result [64]byte, err error) {
	hasher := sha512.New()
	_, err = io.Copy(hasher, stream)
	if err != nil {
		return result, err
	}

	copy(result[:], hasher.Sum(nil))
	return result, nil
}

// GetFileSha512 computes a SHA-512 hash sum from an existing file.
func GetFileSha512(path string) (result [64]byte, err error) {
	reader, err := os.Open(path)
	if err != nil {
		return result, err
	}
	defer func() {
		err = errors.Join(err, reader.Close())
	}()

	return GetSha512(reader)
}
