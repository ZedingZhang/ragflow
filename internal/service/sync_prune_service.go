//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package service

import (
	"context"
	"errors"
	"iter"
	"maps"
)

// ErrSyncDocumentDeleterNotConfigured reports a missing production delete service.
var ErrSyncDocumentDeleterNotConfigured = errors.New("sync document deleter is not configured")

// SyncDocumentDeleter removes documents through the owning business service.
type SyncDocumentDeleter interface {
	// DeleteDocument removes a document and dependent business state.
	DeleteDocument(ctx context.Context, docID string) error
}

// SyncPruneService owns stale document deletion for complete prune snapshots.
type SyncPruneService struct {
	deleter SyncDocumentDeleter
	store   DocumentStore
}

// NewSyncPruneService creates a prune service.
func NewSyncPruneService(deleter SyncDocumentDeleter, store DocumentStore) *SyncPruneService {
	if store == nil {
		store = NewGormDocumentStore()
	}
	return &SyncPruneService{deleter: deleter, store: store}
}

// DeleteStale removes documents missing from the complete source snapshot.
func (s *SyncPruneService) DeleteStale(ctx context.Context, taskContext SyncTaskContext, retain map[string]struct{}, previousSourceIDs map[string]string) (int64, error) {
	sourceType := SourceType(taskContext.Connector.Source, taskContext.Connector.ID)
	kbID := taskContext.Knowledgebase.ID
	var existingIDs iter.Seq[string]
	if len(previousSourceIDs) == 0 {
		ids, err := s.store.ListIDs(ctx, kbID, sourceType)
		if err != nil {
			return 0, err
		}
		existingIDs = maps.Keys(ids)
	} else {
		fingerprints, err := s.store.ListFingerprintsBySourceType(ctx, kbID, sourceType)
		if err != nil {
			return 0, err
		}
		retainUnmigratedDocuments(kbID, taskContext.Connector.ID, fingerprints, retain, previousSourceIDs)
		existingIDs = maps.Keys(fingerprints)
	}
	var removed int64
	for docID := range existingIDs {
		if _, ok := retain[docID]; ok {
			continue
		}
		if s.deleter == nil {
			return removed, ErrSyncDocumentDeleterNotConfigured
		}
		if err := s.deleter.DeleteDocument(ctx, docID); err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}

// One old SQL source ID may represent several tables. Keep it if any live
// replacement is missing or its content fingerprint has not been committed.
func retainUnmigratedDocuments(kbID, connectorID string, fingerprints map[string]string, retain map[string]struct{}, previousSourceIDs map[string]string) {
	for sourceID, previous := range previousSourceIDs {
		legacyID, newID := syncDocumentIDs(kbID, connectorID, sourceID)
		if fingerprints[legacyID] == "" && fingerprints[newID] == "" {
			AddRetainDocumentID(retain, kbID, connectorID, previous)
		}
	}
}

// RetainDocumentIDs calculates PRUNE retain IDs using both legacy and new schemes.
func RetainDocumentIDs(kbID, connectorID string, sourceIDs []string) map[string]struct{} {
	retain := make(map[string]struct{}, len(sourceIDs)*2)
	for _, sourceID := range sourceIDs {
		AddRetainDocumentID(retain, kbID, connectorID, sourceID)
	}
	return retain
}

// AddRetainDocumentID adds both legacy and current document IDs for one source ID.
func AddRetainDocumentID(retain map[string]struct{}, kbID, connectorID, sourceID string) {
	legacyID, newID := syncDocumentIDs(kbID, connectorID, sourceID)
	retain[legacyID] = struct{}{}
	retain[newID] = struct{}{}
}
