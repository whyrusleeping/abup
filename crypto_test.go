package main

import (
	"bytes"
	"io/ioutil"
	"math/rand"
	"testing"
)

func TestEncrypt(t *testing.T) {
	data := make([]byte, 1024)
	rand.Read(data)

	r := bytes.NewReader(data)

	key := []byte("my kitty is a good kitty")
	er, err := encryptReader(key, r)
	if err != nil {
		t.Fatal(err)
	}

	ciphertext, err := ioutil.ReadAll(er)
	if err != nil {
		t.Fatal(err)
	}

	if bytes.Equal(ciphertext, data) {
		t.Fatal("seriously?")
	}

	dr, err := decryptReader(key, bytes.NewReader(ciphertext))
	if err != nil {
		t.Fatal(err)
	}

	out, err := ioutil.ReadAll(dr)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(out, data) {
		t.Fatal("i'm a failure")
	}
}
