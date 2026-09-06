//go:build windows

package config

import (
	"encoding/base64"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	cryptProtectUIForbidden  = 0x1
	cryptProtectLocalMachine = 0x4
)

func secureSecretStorage() bool { return true }
func protectionName() string    { return "windows-dpapi-machine" }

func dataBlob(data []byte) windows.DataBlob {
	if len(data) == 0 {
		return windows.DataBlob{}
	}
	return windows.DataBlob{Size: uint32(len(data)), Data: &data[0]}
}

func blobBytes(blob windows.DataBlob) []byte {
	if blob.Size == 0 || blob.Data == nil {
		return nil
	}
	return append([]byte(nil), unsafe.Slice(blob.Data, int(blob.Size))...)
}

func protectSecret(value string) (string, error) {
	inputBytes := []byte(value)
	input := dataBlob(inputBytes)
	var output windows.DataBlob
	if err := windows.CryptProtectData(&input, nil, nil, 0, nil, cryptProtectUIForbidden|cryptProtectLocalMachine, &output); err != nil {
		return "", err
	}
	defer windows.LocalFree(windows.Handle(uintptr(unsafe.Pointer(output.Data))))
	return base64.StdEncoding.EncodeToString(blobBytes(output)), nil
}

func unprotectSecret(value string) (string, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return "", fmt.Errorf("decode DPAPI blob: %w", err)
	}
	input := dataBlob(ciphertext)
	var output windows.DataBlob
	if err := windows.CryptUnprotectData(&input, nil, nil, 0, nil, cryptProtectUIForbidden, &output); err != nil {
		return "", err
	}
	defer windows.LocalFree(windows.Handle(uintptr(unsafe.Pointer(output.Data))))
	return string(blobBytes(output)), nil
}
