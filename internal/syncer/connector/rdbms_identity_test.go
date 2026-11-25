package connector

import (
	"errors"
	"testing"
)

func TestRDBMSIdentityEscapesTableAndRowBoundaries(t *testing.T) {
	first := rdbmsRowIdentity("mysql", "mydb", "a/b", "c")
	second := rdbmsRowIdentity("mysql", "mydb", "a", "b/c")
	literal := rdbmsRowIdentity("mysql", "mydb", "a%2Fb", "c")
	if first.SourceID == second.SourceID || first.SourceID == literal.SourceID {
		t.Fatalf("ambiguous source IDs: %q, %q, %q", first.SourceID, second.SourceID, literal.SourceID)
	}
}

func TestRDBMSTablesScopeContentFallback(t *testing.T) {
	row := map[string]any{"title": "same content"}
	columns := []string{"title"}
	for _, identity := range []func(string, map[string]any, []string) SlimDocument{
		(&MySQLConnector{database: "db"}).rowIdentity,
		(&PostgreSQLConnector{database: "db"}).rowIdentity,
	} {
		a, b := identity("a", row, columns), identity("b", row, columns)
		custom := identity("", row, columns)
		if a.SourceID == b.SourceID || a.PreviousSourceID != custom.SourceID || b.PreviousSourceID != custom.SourceID || custom.PreviousSourceID != "" {
			t.Fatalf("unexpected table and custom query IDs: %+v, %+v, %+v", a, b, custom)
		}
	}
}

func TestRDBMSResumeRestartsPreUpgradeTableWindow(t *testing.T) {
	mysql := mysqlSyncSession{queryNames: []string{"orders"}, orderColumn: "id"}
	postgres := postgresSyncSession{queryNames: []string{"orders"}, orderColumn: "id"}
	for dialect, resume := range map[string]func(*SyncCheckpoint) error{
		"mysql": mysql.applyResume, "postgresql": postgres.applyResume,
	} {
		checkpoint := &SyncCheckpoint{Cursor: encodeRDBMSCursor("orders", "id", dialect+":db:1")}
		if err := resume(checkpoint); !errors.Is(err, ErrSyncResumeInvalid) {
			t.Fatalf("%s: resume error = %v, want window restart", dialect, err)
		}
	}
}
