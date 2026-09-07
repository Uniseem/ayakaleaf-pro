package rediskeys

import "testing"

const (
	docID     = "6a9eb3bd8a19695a4a6c97ac"
	projectID = "6a9eb3bc8a19695a4a6c97a7"
	clientID  = "P.d2Rg3yoA1YYPA3owrb1a"
)

// The two tables below are transcribed from the two settings files that define
// them. They are the whole point of this package: a name that is wrong here
// fails silently, so it is spelled out rather than derived.
//
//	upstream: services/*/config/settings.defaults.js
//	server-ce: server-ce/config/settings.js, merged over those defaults
func TestKeyNames(t *testing.T) {
	cases := []struct {
		name           string
		key            func(Schema) string
		upstream       string
		serverCE       string
		overriddenByCE bool
	}{
		// The nine keys server-ce redefines, which lose their braces.
		{"BlockingKey", func(s Schema) string { return s.BlockingKey(docID) },
			"Blocking:{" + docID + "}", "Blocking:" + docID, true},
		{"DocLines", func(s Schema) string { return s.DocLines(docID) },
			"doclines:{" + docID + "}", "doclines:" + docID, true},
		{"DocOps", func(s Schema) string { return s.DocOps(docID) },
			"DocOps:{" + docID + "}", "DocOps:" + docID, true},
		{"DocVersion", func(s Schema) string { return s.DocVersion(docID) },
			"DocVersion:{" + docID + "}", "DocVersion:" + docID, true},
		{"DocHash", func(s Schema) string { return s.DocHash(docID) },
			"DocHash:{" + docID + "}", "DocHash:" + docID, true},
		{"ProjectKey", func(s Schema) string { return s.ProjectKey(docID) },
			"ProjectId:{" + docID + "}", "ProjectId:" + docID, true},
		{"DocsInProject", func(s Schema) string { return s.DocsInProject(projectID) },
			"DocsIn:{" + projectID + "}", "DocsIn:" + projectID, true},
		{"Ranges", func(s Schema) string { return s.Ranges(docID) },
			"Ranges:{" + docID + "}", "Ranges:" + docID, true},
		{"PendingUpdates", func(s Schema) string { return s.PendingUpdates(docID) },
			"PendingUpdates:{" + docID + "}", "PendingUpdates:" + docID, true},
		{"ClientsInProject", func(s Schema) string { return s.ClientsInProject(projectID) },
			"clients_in_project:{" + projectID + "}", "clients_in_project:" + projectID, true},
		{"ConnectedUser", func(s Schema) string { return s.ConnectedUser(projectID, clientID) },
			"connected_user:{" + projectID + "}:" + clientID,
			"connected_user:" + projectID + ":" + clientID, true},

		// The keys server-ce leaves alone, which keep their braces under both.
		{"UnflushedTime", func(s Schema) string { return s.UnflushedTime(docID) },
			"UnflushedTime:{" + docID + "}", "UnflushedTime:{" + docID + "}", false},
		{"Pathname", func(s Schema) string { return s.Pathname(docID) },
			"Pathname:{" + docID + "}", "Pathname:{" + docID + "}", false},
		{"ProjectHistoryID", func(s Schema) string { return s.ProjectHistoryID(docID) },
			"ProjectHistoryId:{" + docID + "}", "ProjectHistoryId:{" + docID + "}", false},
		{"ProjectState", func(s Schema) string { return s.ProjectState(projectID) },
			"ProjectState:{" + projectID + "}", "ProjectState:{" + projectID + "}", false},
		{"ProjectBlock", func(s Schema) string { return s.ProjectBlock(projectID) },
			"ProjectBlock:{" + projectID + "}", "ProjectBlock:{" + projectID + "}", false},
		{"LastUpdatedAt", func(s Schema) string { return s.LastUpdatedAt(docID) },
			"lastUpdatedAt:{" + docID + "}", "lastUpdatedAt:{" + docID + "}", false},
		{"LastUpdatedBy", func(s Schema) string { return s.LastUpdatedBy(docID) },
			"lastUpdatedBy:{" + docID + "}", "lastUpdatedBy:{" + docID + "}", false},
		{"ResolvedCommentIds", func(s Schema) string { return s.ResolvedCommentIds(docID) },
			"ResolvedCommentIds:{" + docID + "}", "ResolvedCommentIds:{" + docID + "}", false},
		{"ProjectNotificationTimestamp",
			func(s Schema) string { return s.ProjectNotificationTimestamp(projectID) },
			"ProjectNotificationTimestamp:{" + projectID + "}",
			"ProjectNotificationTimestamp:{" + projectID + "}", false},
		{"ProjectNotEmptySince", func(s Schema) string { return s.ProjectNotEmptySince(projectID) },
			"projectNotEmptySince:{" + projectID + "}",
			"projectNotEmptySince:{" + projectID + "}", false},

		{"ProjectHistoryOps", func(s Schema) string { return s.ProjectHistoryOps(projectID) },
			"ProjectHistory:Ops:{" + projectID + "}",
			"ProjectHistory:Ops:{" + projectID + "}", false},
		{"ProjectHistoryFirstOpTimestamp",
			func(s Schema) string { return s.ProjectHistoryFirstOpTimestamp(projectID) },
			"ProjectHistory:FirstOpTimestamp:{" + projectID + "}",
			"ProjectHistory:FirstOpTimestamp:{" + projectID + "}", false},

		// Keys with no id, identical under both.
		{"HistoryRangesSupport", func(s Schema) string { return s.HistoryRangesSupport() },
			"HistoryRangesSupport", "HistoryRangesSupport", false},
		{"FlushAndDeleteQueue", func(s Schema) string { return s.FlushAndDeleteQueue() },
			"DocUpdaterFlushAndDeleteQueue", "DocUpdaterFlushAndDeleteQueue", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.key(Upstream); got != tc.upstream {
				t.Errorf("upstream = %q, want %q", got, tc.upstream)
			}
			if got := tc.key(ServerCE); got != tc.serverCE {
				t.Errorf("server-ce = %q, want %q", got, tc.serverCE)
			}
			// A key that server-ce overrides must differ between the two, and
			// one it leaves alone must not. Getting this backwards for a single
			// key is the whole failure mode.
			differ := tc.upstream != tc.serverCE
			if differ != tc.overriddenByCE {
				t.Errorf("the two schemas %s for this key, which is wrong",
					map[bool]string{true: "differ", false: "agree"}[differ])
			}
		})
	}
}

func TestNewSelectsSchema(t *testing.T) {
	if !New("server-ce").IsServerCE() {
		t.Error(`New("server-ce") should select the server-ce schema`)
	}
	// An unrecognised name must not silently pick the wrong schema.
	for _, name := range []string{"", "upstream", "nonsense", "server_ce", "SERVER-CE"} {
		if New(name).IsServerCE() {
			t.Errorf("New(%q) should fall back to the upstream schema", name)
		}
	}
}
