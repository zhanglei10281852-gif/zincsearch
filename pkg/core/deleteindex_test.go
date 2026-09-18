/* Copyright 2022 Zinc Labs Inc. and Contributors
*
* Licensed under the Apache License, Version 2.0 (the "License");
* you may not use this file except in compliance with the License.
* You may obtain a copy of the License at
*
*     http://www.apache.org/licenses/LICENSE-2.0
*
* Unless required by applicable law or agreed to in writing, software
* distributed under the License is distributed on an "AS IS" BASIS,
* WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
* See the License for the specific language governing permissions and
* limitations under the License.
 */

package core

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zincsearch/zincsearch/pkg/metadata"
	zincErrors "github.com/zincsearch/zincsearch/pkg/errors"
)

func storeTestIndex(t *testing.T, name string) {
	t.Helper()
	index, err := NewIndex(name, "disk", 1)
	require.NoError(t, err)
	require.NotNil(t, index)
	require.NoError(t, StoreIndex(index))
}

func TestDeleteIndex(t *testing.T) {
	indexName := "TestDeleteIndex.index_1"
	type args struct {
		name string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "exist",
			args: args{
				name: indexName,
			},
			wantErr: false,
		},
		{
			name: "not exist",
			args: args{
				name: "my-index-not-exist",
			},
			wantErr: true,
		},
	}

	t.Run("prepare", func(t *testing.T) {
		index, err := NewIndex(indexName, "disk", 2)
		assert.NoError(t, err)
		assert.NotNil(t, index)
		err = StoreIndex(index)
		assert.NoError(t, err)
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := DeleteIndex(tt.args.name); (err != nil) != tt.wantErr {
				t.Errorf("DeleteIndex() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// deleting an index must remove it from aliases both in the running process
// and in the persisted metadata, so after a restart alias search can never
// point at the deleted target again.
func TestDeleteIndex_CleansAliasFromMemoryAndMetadata(t *testing.T) {
	keep := "TestDeleteIndex_Alias.keep"
	gone := "TestDeleteIndex_Alias.gone"
	aliasName := "TestDeleteIndex_Alias.daily"

	storeTestIndex(t, keep)
	storeTestIndex(t, gone)
	t.Cleanup(func() { _ = DeleteIndex(keep) })

	require.NoError(t, ZINC_INDEX_ALIAS_LIST.AddIndexesToAlias(aliasName, []string{keep, gone}))

	require.NoError(t, DeleteIndex(gone))

	// current process: the alias only resolves to the surviving index
	indexes, ok := ZINC_INDEX_ALIAS_LIST.GetIndexesForAlias(aliasName)
	require.True(t, ok)
	require.Equal(t, []string{keep}, indexes)

	// persisted metadata (what a restart would load) says the same
	stored, err := metadata.Alias.Get()
	require.NoError(t, err)
	require.Equal(t, []string{keep}, stored[aliasName])

	// cache, index metadata and physical directory are all gone
	_, exists := GetIndex(gone)
	require.False(t, exists)

	_, err = metadata.Index.Get(gone)
	require.ErrorIs(t, err, zincErrors.ErrKeyNotFound)

	_, err = os.Stat(indexDataPath(gone))
	require.True(t, os.IsNotExist(err))
}

// if deleting the index metadata fails, nothing is published: the index
// stays cached, its alias membership stays visible and persisted, and the
// same request retries successfully.
func TestDeleteIndex_RollbackWhenIndexMetadataDeleteFails(t *testing.T) {
	mem := withMemMetadata(t)
	name := "TestDeleteIndex_MetaFail.name"
	aliasName := "TestDeleteIndex_MetaFail.daily"

	storeTestIndex(t, name)
	require.NoError(t, ZINC_INDEX_ALIAS_LIST.AddIndexesToAlias(aliasName, []string{name}))

	mem.FailDelete = func(key string) error {
		if key == "/index/"+name {
			return errors.New("metadata delete failed")
		}
		return nil
	}

	err := DeleteIndex(name)
	require.Error(t, err)

	_, ok := GetIndex(name)
	require.True(t, ok, "index must stay cached after failed metadata deletion")

	data, err := metadata.Index.Get(name)
	require.NoError(t, err)
	require.NotEmpty(t, data, "index metadata must still exist after failed deletion")

	require.Contains(t, ZINC_INDEX_ALIAS_LIST.GetAliasesForIndex(name), aliasName)
	stored, err := metadata.Alias.Get()
	require.NoError(t, err)
	require.Contains(t, stored[aliasName], name)

	// retry the original request after recovery
	mem.FailDelete = nil
	require.NoError(t, DeleteIndex(name))

	_, ok = GetIndex(name)
	require.False(t, ok)
	_, err = metadata.Index.Get(name)
	require.Error(t, err, "index metadata must be gone after successful retry")
	_, ok = ZINC_INDEX_ALIAS_LIST.GetIndexesForAlias(aliasName)
	require.False(t, ok)
}

// if persisting the detached aliases fails after the index metadata was
// deleted, the index metadata is restored as a compensating action and the
// deletion can be retried as-is.
func TestDeleteIndex_RestoresMetadataWhenAliasPersistFails(t *testing.T) {
	mem := withMemMetadata(t)
	name := "TestDeleteIndex_AliasPersistFail.name"
	aliasName := "TestDeleteIndex_AliasPersistFail.daily"

	storeTestIndex(t, name)
	require.NoError(t, ZINC_INDEX_ALIAS_LIST.AddIndexesToAlias(aliasName, []string{name}))

	mem.FailSet = func(key string) error {
		if key == "/aliases/alias" {
			return errors.New("alias metadata write failed")
		}
		return nil
	}

	err := DeleteIndex(name)
	require.Error(t, err)

	// compensating restore brought the index metadata back
	data, err := metadata.Index.Get(name)
	require.NoError(t, err)
	require.NotEmpty(t, data)

	// cache and alias membership stay visible
	_, ok := GetIndex(name)
	require.True(t, ok)
	require.Contains(t, ZINC_INDEX_ALIAS_LIST.GetAliasesForIndex(name), aliasName)
	stored, err := metadata.Alias.Get()
	require.NoError(t, err)
	require.Contains(t, stored[aliasName], name)

	// retry the original request after recovery
	mem.FailSet = nil
	require.NoError(t, DeleteIndex(name))

	_, ok = GetIndex(name)
	require.False(t, ok)
	_, err = metadata.Index.Get(name)
	require.Error(t, err, "index metadata must be gone after successful retry")
	_, ok = ZINC_INDEX_ALIAS_LIST.GetIndexesForAlias(aliasName)
	require.False(t, ok)
}

// when physical directory removal fails after the deletion was committed,
// the concrete error is returned while metadata/cache/aliases already agree
// the index is gone; retrying the deletion cleans the leftover directory.
func TestDeleteIndex_PhysicalFailureLeavesRecoverableState(t *testing.T) {
	name := "TestDeleteIndex_PhysFail.name"
	aliasName := "TestDeleteIndex_PhysFail.daily"

	storeTestIndex(t, name)
	require.NoError(t, ZINC_INDEX_ALIAS_LIST.AddIndexesToAlias(aliasName, []string{name}))

	dir := indexDataPath(name)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "segment.lock"), []byte("locked"), 0o644))

	origRemoveAll := removeAll
	removeAll = func(string) error { return errors.New("device busy") }
	err := DeleteIndex(name)
	removeAll = origRemoveAll
	require.Error(t, err)
	require.Contains(t, err.Error(), name)

	// logical state is committed: no layer can reach the deleted index
	_, ok := GetIndex(name)
	require.False(t, ok)
	_, err = metadata.Index.Get(name)
	require.ErrorIs(t, err, zincErrors.ErrKeyNotFound)
	require.Empty(t, ZINC_INDEX_ALIAS_LIST.GetAliasesForIndex(name))
	stored, err := metadata.Alias.Get()
	require.NoError(t, err)
	require.NotContains(t, stored, aliasName)

	// only the physical leftovers remain
	_, err = os.Stat(dir)
	require.NoError(t, err, "data directory should still be present")

	// retry performs the idempotent cleanup
	require.NoError(t, DeleteIndex(name))
	_, err = os.Stat(dir)
	require.True(t, os.IsNotExist(err))
}

// concurrent alias updates and an index deletion must serialize into one
// committed target set: no lost updates, no duplicates, and the deleted
// index can never remain an alias target.
func TestDeleteIndex_ConcurrentAliasChanges(t *testing.T) {
	victim := "TestDeleteIndex_Concurrent.victim"
	aliasName := "TestDeleteIndex_Concurrent.daily"
	const n = 20

	survivors := make([]string, 0, n)
	for i := 0; i < n; i++ {
		survivor := "TestDeleteIndex_Concurrent.surv_" + string(rune('a'+i))
		storeTestIndex(t, survivor)
		survivors = append(survivors, survivor)
	}
	t.Cleanup(func() {
		for _, survivor := range survivors {
			_ = DeleteIndex(survivor)
		}
	})
	storeTestIndex(t, victim)
	require.NoError(t, ZINC_INDEX_ALIAS_LIST.AddIndexesToAlias(aliasName, []string{victim}))

	var wg sync.WaitGroup
	for i, survivor := range survivors {
		wg.Add(1)
		go func(i int, survivor string) {
			defer wg.Done()
			if i%2 == 0 {
				_ = ZINC_INDEX_ALIAS_LIST.AddIndexesToAlias(aliasName, []string{survivor})
			} else {
				_ = ZINC_INDEX_ALIAS_LIST.ApplyMembers(map[string][]string{aliasName: {survivor}}, nil)
			}
		}(i, survivor)
	}

	// an add racing the deletion either precedes it (then gets detached) or
	// is skipped because the index already left the cache - either order
	// converges to a set without the victim
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = ZINC_INDEX_ALIAS_LIST.ApplyMembers(map[string][]string{aliasName: {victim}}, nil)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = DeleteIndex(victim)
	}()

	wg.Wait()

	indexes, ok := ZINC_INDEX_ALIAS_LIST.GetIndexesForAlias(aliasName)
	require.True(t, ok)
	require.Len(t, indexes, n)

	committed := make(map[string]struct{}, n)
	for _, index := range indexes {
		require.NotEqual(t, victim, index, "deleted index must never remain an alias target")
		committed[index] = struct{}{}
	}
	require.Len(t, committed, n, "committed target set must contain every survivor exactly once")

	// memory and persisted metadata agree on the committed set
	stored, err := metadata.Alias.Get()
	require.NoError(t, err)
	require.ElementsMatch(t, indexes, stored[aliasName])

	_, exists := GetIndex(victim)
	require.False(t, exists)
	_, err = metadata.Index.Get(victim)
	require.ErrorIs(t, err, zincErrors.ErrKeyNotFound)
}
