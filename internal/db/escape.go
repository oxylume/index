package db

import "strings"

var likeReplacer = strings.NewReplacer("%", "\\%", "_", "\\_", "\\", "\\\\")

func escapeLikeSearch(term string) string {
	return likeReplacer.Replace(term)
}
