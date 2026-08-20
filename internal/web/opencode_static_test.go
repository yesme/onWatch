package web

import (
	"strings"
	"testing"
)

func TestAppJS_OpenCodeQuotaCardsRerenderWhenQuotaSetChanges(t *testing.T) {
	t.Parallel()

	appJS := readStaticAppJS(t)
	if !strings.Contains(appJS, "function openCodeQuotaSetsMatch(container, quotas)") {
		t.Fatal("OpenCode cards must compare rendered and incoming quota-name sets")
	}

	updateIdx := strings.Index(appJS, "} else if (provider === 'opencode') {")
	if updateIdx < 0 {
		t.Fatal("OpenCode current-usage update branch not found")
	}
	updateBody := appJS[updateIdx:]
	setMatchIdx := strings.Index(updateBody, "!openCodeQuotaSetsMatch(container, data.quotas)")
	updateCardsIdx := strings.Index(updateBody, "data.quotas.forEach(q => updateOpenCodeCard(q));")
	if setMatchIdx < 0 {
		t.Fatal("OpenCode cards must re-render when quota-name sets differ")
	}
	if updateCardsIdx < 0 || setMatchIdx > updateCardsIdx {
		t.Fatal("OpenCode quota set comparison must happen before in-place card updates")
	}
}
