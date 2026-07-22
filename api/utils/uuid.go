package utils

import (
	"crypto/rand"
	"io"
	"fmt"
)

// NewUUID 生成标准 RFC4122 Version 4 随机UUID
func NewUUID() (string, error) {
	uuid := make([]byte, 16)
	n, err := io.ReadFull(rand.Reader, uuid)
	if n != len(uuid) || err != nil {
		return "", err
	}

	// 设置variant标识位 (RFC4122 4.1.1)
	uuid[8] = uuid[8] &^ 0xc0 | 0x80
	// 设置version 4 随机标识位 (RFC4122 4.1.3)
	uuid[6] = uuid[6] &^ 0xf0 | 0x40

	// 按标准格式拼接 8-4-4-4-12
	return fmt.Sprintf("%x-%x-%x-%x-%x",
		uuid[0:4],
		uuid[4:6],
		uuid[6:8],
		uuid[8:10],
		uuid[10:16]), nil
}