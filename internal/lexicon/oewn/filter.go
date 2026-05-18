package oewn

import "regexp"

var lemmaRE = regexp.MustCompile(`^[A-Za-z']+[A-Za-z' -]*$`)

func AllowLemma(lemma string) bool {
	return lemmaRE.MatchString(lemma)
}
