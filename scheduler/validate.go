package main

import "regexp"

var uuidRe = regexp.MustCompile(
		`^[0-9a-fA-F]{8}-[0-9a-fA-F]
{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`,
)

func validVideoID(vid string) bool{
	return uuidRe.MatchString(vid)
}