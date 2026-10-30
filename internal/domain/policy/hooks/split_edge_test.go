package hooks

import (
	"cmp"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/platform/engines"
	"gitlab.com/dev.chempa/agentry/internal/platform/hookcall"
)

// A split whose total is 0 is no count: the turn says why, as before, and
// keeps no split of zeros.
func TestASplitOfZerosIsNoCount(t *testing.T) {
	for _, ending := range []*hookcall.Ending{
		{Tokens: 0, Split: &engines.Tokens{}, TokensUnknown: engines.NoUsageThisTurn},
		{Tokens: 0, Split: &engines.Tokens{}},
	} {
		data := turnEnded(t, ending)
		if want := cmp.Or(ending.TokensUnknown, NoTokensReported); data["tokensUnknown"] != want {
			t.Errorf("tokensUnknown %v, want %q (data %v)", data["tokensUnknown"], want, data)
		}
		for _, field := range []string{"inputTokens", "cacheWriteTokens", "cacheReadTokens", "outputTokens"} {
			if _, ok := data[field]; ok {
				t.Errorf("a turn with no count has %s: %v", field, data)
			}
		}
	}
}
