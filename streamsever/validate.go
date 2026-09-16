package main

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var uuidRe = regexp.MustCompile(
		`^[0-9a-fA-F]{8}-[0-9a-fA-F]
{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`,
)

func ValidVideoID(vid string) bool{
	return uuidRe.MatchString(vid)
}

func SafePath(dir string, name string) (string, error){
	full := filepath.Join(dir, name)
	base := filepath.Clean(dir) + string(filepath.Separator)

	if !strings.HasPrefix(filepath.Clean(full), base){
		return "", fmt.Errorf("path %q escapes base dir %q", name, dir)
	}
	return full, nil
}