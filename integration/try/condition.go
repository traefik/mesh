package try

import (
	"strings"
)

type List []string

func (l List) contains(s string) bool {
	for _, v := range l {
		if strings.Contains(s, v) {
			return true
		}
	}
	return false
}
