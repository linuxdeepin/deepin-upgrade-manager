package sha256

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
)

type SHA256 struct{}

func (handler *SHA256) Sign(data []byte) ([]byte, error) {
	digest := sha256.Sum256(data)
	return digest[:], nil
}

func (handler *SHA256) SignFile(filename string) ([]byte, error) {
	fr, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer fr.Close()

	// TODO(jouyouyun): sum by chunk
	var h = sha256.New()
	_, err = io.Copy(h, fr)
	if err != nil {
		return nil, err
	}

	return h.Sum(nil), nil
}

func (handler *SHA256) Verify(data []byte, signed string) (bool, error) {
	digest, _ := handler.Sign(data)
	return fmt.Sprintf("%x", digest) == signed, nil
}

func (handler *SHA256) VerifyFile(filename, signed string) (bool, error) {
	digest, err := handler.SignFile(filename)
	if err != nil {
		return false, err
	}
	return fmt.Sprintf("%x", digest) == signed, nil
}
