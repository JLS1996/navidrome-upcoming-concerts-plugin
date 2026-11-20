package radar

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
)

// Only authorization belongs in the scope. Matching/display preferences must not
// invalidate every artist's completed lookup or the direct-credit inventory.
func AccessScope(username string, libraries []int) string {
	ids := append([]int(nil), libraries...)
	sort.Ints(ids)
	b, _ := json.Marshal(struct {
		User      string
		Libraries []int
	}{username, ids})
	return "access:" + digest(string(b))
}
func SameAccessScope(old, username string, libraries []int) bool {
	if old == AccessScope(username, libraries) {
		return true
	}
	at := strings.LastIndex(old, "|[")
	if at < 0 || !strings.HasSuffix(old, "]") {
		return false
	}
	var legacy struct{ User string }
	if json.Unmarshal([]byte(old[:at]), &legacy) != nil || legacy.User != username {
		return false
	}
	ids := []int{}
	for _, field := range strings.Fields(old[at+2 : len(old)-1]) {
		n, err := strconv.Atoi(field)
		if err != nil || n < 1 {
			return false
		}
		ids = append(ids, n)
	}
	return AccessScope(username, ids) == AccessScope(username, libraries)
}
func MigrateScope(scope *string, username string, libraries []int) {
	if SameAccessScope(*scope, username, libraries) {
		*scope = AccessScope(username, libraries)
	}
}
