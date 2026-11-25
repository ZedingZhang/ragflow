package service

import (
	"context"
	"slices"
	"testing"
)

type pruneFingerprintStore struct {
	DocumentStore
	fingerprints map[string]string
}

func (s pruneFingerprintStore) ListFingerprintsBySourceType(context.Context, string, string) (map[string]string, error) {
	return s.fingerprints, nil
}

type pruneRecorder struct{ deleted []string }

func (r *pruneRecorder) DeleteDocument(_ context.Context, id string) error {
	r.deleted = append(r.deleted, id)
	return nil
}

func TestPruneRetainsSharedDocumentUntilAllReplacementsAreCommitted(t *testing.T) {
	var task SyncTaskContext
	task.Knowledgebase.ID, task.Connector.ID = "kb", "conn"
	previous := "mysql:db:1"
	orders, products := "mysql/table/db/orders/1", "mysql/table/db/products/1"
	oldLegacy, oldCurrent := syncDocumentIDs("kb", "conn", previous)
	orderID, _ := syncDocumentIDs("kb", "conn", orders)
	_, productID := syncDocumentIDs("kb", "conn", products)
	for _, tc := range []struct {
		name        string
		hashes      map[string]string
		liveRows    []string
		wantDeletes []string
	}{
		{"before full sync", map[string]string{}, []string{orders, products}, nil},
		{"one table imported", map[string]string{orderID: "done"}, []string{orders, products}, nil},
		{"second table pending", map[string]string{orderID: "done", productID: ""}, []string{orders, products}, nil},
		{"both tables imported", map[string]string{orderID: "done", productID: "done"}, []string{orders, products}, []string{oldLegacy, oldCurrent}},
		{"order removed at source", map[string]string{orderID: "done", productID: "done"}, []string{products}, []string{oldLegacy, oldCurrent, orderID}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.hashes[oldLegacy], tc.hashes[oldCurrent] = "old", "old"
			recorder := &pruneRecorder{}
			svc := NewSyncPruneService(recorder, pruneFingerprintStore{fingerprints: tc.hashes})
			retain := RetainDocumentIDs("kb", "conn", tc.liveRows)
			previousIDs := make(map[string]string)
			for _, row := range tc.liveRows {
				previousIDs[row] = previous
			}
			_, err := svc.DeleteStale(t.Context(), task, retain, previousIDs)
			if err != nil {
				t.Fatal(err)
			}
			slices.Sort(recorder.deleted)
			slices.Sort(tc.wantDeletes)
			if !slices.Equal(recorder.deleted, tc.wantDeletes) {
				t.Fatalf("deleted = %v, want %v", recorder.deleted, tc.wantDeletes)
			}
		})
	}
}
