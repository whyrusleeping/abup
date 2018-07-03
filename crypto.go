package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"io"
)

type cipherReader struct {
	baseR  io.Reader
	stream cipher.Stream
	iv     *bytes.Reader
	readIv bool
}

func encryptReader(key []byte, r io.Reader) (io.Reader, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	iv := make([]byte, aes.BlockSize)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, err
	}

	return &cipherReader{
		baseR:  r,
		stream: cipher.NewCFBEncrypter(block, iv),
		iv:     bytes.NewReader(iv),
	}, nil
}

func (cr *cipherReader) Read(b []byte) (int, error) {
	if !cr.readIv {
		n, err := cr.iv.Read(b)
		switch err {
		case io.EOF:
			cr.readIv = true
		default:
			return n, err
		}
	}
	temp := make([]byte, len(b))
	n, err := cr.baseR.Read(temp)
	cr.stream.XORKeyStream(b[:n], temp[:n])
	return n, err
}

type cipherDecryptReader struct {
	cipherR io.Reader
	stream  cipher.Stream
}

func decryptReader(key []byte, r io.Reader) (io.Reader, error) {
	iv := make([]byte, aes.BlockSize)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	_, err = io.ReadFull(r, iv)
	if err != nil {
		return nil, err
	}

	return &cipherDecryptReader{
		cipherR: r,
		stream:  cipher.NewCFBDecrypter(block, iv),
	}, nil
}

func (cdr *cipherDecryptReader) Read(b []byte) (int, error) {
	n, err := cdr.cipherR.Read(b)
	cdr.stream.XORKeyStream(b[:n], b[:n])
	return n, err
}
