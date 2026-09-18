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
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zincsearch/zincsearch/pkg/metadata"
	"github.com/zincsearch/zincsearch/test/utils"
)

// withMemMetadata swaps the metadata storage for an in-memory one that can
// inject failures; the previous backend is restored when the test ends.
func withMemMetadata(t *testing.T) *utils.MemStorage {
	t.Helper()
	mem := utils.NewMemStorage()
	prev := metadata.SetDB(mem)
	t.Cleanup(func() { metadata.SetDB(prev) })
	return mem
}

func TestAliasList_AddIndexesToAlias(t *testing.T) {
	type args struct {
		alias   string
		indexes []string
	}
	tests := []struct {
		name        string
		nFn         func(al *AliasList)
		args        args
		wantErr     bool
		wantIndexes []string
	}{
		{
			name: "should_add_indexes_to_alias",
			nFn: func(al *AliasList) {
				al.Aliases["alias_1"] = append(al.Aliases["alias_1"], "index_0")
			},
			args: args{
				alias:   "alias_1",
				indexes: []string{"index_1", "index_2"},
			},
			wantErr:     false,
			wantIndexes: []string{"index_0", "index_1", "index_2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			al := NewAliasList()

			if tt.nFn != nil {
				tt.nFn(al)
			}

			err := al.AddIndexesToAlias(tt.args.alias, tt.args.indexes)
			if tt.wantErr {
				require.NotNil(t, err)
				return
			}

			require.Equal(t, tt.wantIndexes, al.Aliases[tt.args.alias])
		})
	}
}

func TestAliasList_RemoveIndexesFromAlias(t *testing.T) {
	type args struct {
		alias         string
		removeIndexes []string
	}
	tests := []struct {
		name        string
		nFn         func(al *AliasList)
		args        args
		wantErr     bool
		wantIndexes []string
	}{
		{
			name: "should_remove_indexes_from_alias",
			nFn: func(al *AliasList) {
				al.Aliases["alias_1"] = append(al.Aliases["alias_1"], "index_0", "index_1", "index_2", "index_3")
			},
			args: args{
				alias:         "alias_1",
				removeIndexes: []string{"index_1", "index_3"},
			},
			wantErr:     false,
			wantIndexes: []string{"index_0", "index_2"},
		},
		{
			name: "should_not_find_alias",
			nFn:  nil,
			args: args{
				alias:         "alias_1",
				removeIndexes: []string{"index_1"},
			},
			wantErr:     false,
			wantIndexes: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			al := NewAliasList()

			if tt.nFn != nil {
				tt.nFn(al)
			}

			err := al.RemoveIndexesFromAlias(tt.args.alias, tt.args.removeIndexes)
			if tt.wantErr {
				require.NotNil(t, err)
				return
			}

			require.Equal(t, tt.wantIndexes, al.Aliases[tt.args.alias])
		})
	}
}

func TestAliasList_GetIndexesForAlias(t *testing.T) {
	type args struct {
		aliasName string
	}

	tests := []struct {
		name        string
		nFn         func(al *AliasList)
		args        args
		wantOk      bool
		wantIndexes []string
	}{
		{
			name: "should_get_indexes_for_alias",
			nFn: func(al *AliasList) {
				al.Aliases["alias_1"] = append(al.Aliases["alias_1"], "index_0", "index_1", "index_2")
			},
			args: args{
				aliasName: "alias_1",
			},
			wantOk:      true,
			wantIndexes: []string{"index_0", "index_1", "index_2"},
		},
		{
			name: "should_get_no_indexes_for_alias",
			nFn:  nil,
			args: args{
				aliasName: "alias_1",
			},
			wantOk:      false,
			wantIndexes: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			al := NewAliasList()

			if tt.nFn != nil {
				tt.nFn(al)
			}

			indexes, ok := al.GetIndexesForAlias(tt.args.aliasName)

			require.Equal(t, tt.wantOk, ok)
			require.Equal(t, tt.wantIndexes, indexes)
		})
	}
}

func TestAliasList_GetAliasesForIndex(t *testing.T) {
	type args struct {
		indexName string
	}

	tests := []struct {
		name        string
		nFn         func(al *AliasList)
		args        args
		wantAliases []string
	}{
		{
			name: "should_get_aliases_for_index",
			nFn: func(al *AliasList) {
				al.Aliases["alias_1"] = append(al.Aliases["alias_1"], "index_0", "index_1", "index_2")
				al.Aliases["alias_2"] = append(al.Aliases["alias_2"], "index_0", "index_1", "index_2")
			},
			args: args{
				indexName: "index_0",
			},
			wantAliases: []string{"alias_1", "alias_2"},
		},
		{
			name: "should_get_no_indexes_for_alias",
			nFn: func(al *AliasList) {
				al.Aliases["alias_1"] = append(al.Aliases["alias_1"], "index_0", "index_1", "index_2")
				al.Aliases["alias_2"] = append(al.Aliases["alias_1"], "index_0", "index_1", "index_2")
			},
			args: args{
				indexName: "index_6",
			},
			wantAliases: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			al := NewAliasList()

			if tt.nFn != nil {
				tt.nFn(al)
			}

			indexes := al.GetAliasesForIndex(tt.args.indexName)
			require.ElementsMatch(t, tt.wantAliases, indexes)
		})
	}
}

func TestAliasList_GetAliasMap(t *testing.T) {
	type args struct {
		targetIndexes []string
		targetAliases []string
	}
	tests := []struct {
		name string
		nFn  func(al *AliasList)
		args args
		want M
	}{
		{
			name: "should_get_alias_map",
			nFn: func(al *AliasList) {
				al.Aliases["alias_1"] = append(al.Aliases["alias_1"], "index_0")
				al.Aliases["alias_2"] = append(al.Aliases["alias_2"], "index_1")
			},
			args: args{
				targetIndexes: nil,
				targetAliases: nil,
			},
			want: M{
				"index_0": M{
					"aliases": M{
						"alias_1": struct{}{},
					},
				},
				"index_1": M{
					"aliases": M{
						"alias_2": struct{}{},
					},
				},
			},
		},
		{
			name: "should_get_alias_map_with_targets",
			nFn: func(al *AliasList) {
				al.Aliases["alias_1"] = append(al.Aliases["alias_1"], "index_0")
				al.Aliases["alias_2"] = append(al.Aliases["alias_2"], "index_1")
			},
			args: args{
				targetIndexes: []string{"index_1"},
				targetAliases: []string{"alias_2"},
			},
			want: M{
				"index_1": M{
					"aliases": M{
						"alias_2": struct{}{},
					},
				},
			},
		},
		{
			name: "should_get_alias_map_with_targets",
			nFn: func(al *AliasList) {
				al.Aliases["alias_1"] = append(al.Aliases["alias_1"], "index_0")
				al.Aliases["alias_2"] = append(al.Aliases["alias_2"], "index_1")
			},
			args: args{
				targetIndexes: []string{"index_0"},
				targetAliases: []string{"alias_1"},
			},
			want: M{
				"index_0": M{
					"aliases": M{
						"alias_1": struct{}{},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			al := NewAliasList()

			if tt.nFn != nil {
				tt.nFn(al)
			}

			m := al.GetAliasMap(tt.args.targetIndexes, tt.args.targetAliases)
			require.Equal(t, tt.want, m)
		})
	}
}

func TestAliasList_AddRollbackAndRetryOnPersistError(t *testing.T) {
	mem := withMemMetadata(t)
	al := NewAliasList()

	require.NoError(t, al.AddIndexesToAlias("logs_daily", []string{"logs_1"}))

	// a metadata write failure must keep the previously visible state
	mem.FailSet = func(string) error { return errors.New("metadata write failed") }
	err := al.AddIndexesToAlias("logs_daily", []string{"logs_2"})
	require.Error(t, err)

	indexes, ok := al.GetIndexesForAlias("logs_daily")
	require.True(t, ok)
	require.Equal(t, []string{"logs_1"}, indexes)

	// the persisted state must not contain half of the failed add
	stored, err := metadata.Alias.Get()
	require.NoError(t, err)
	require.Equal(t, []string{"logs_1"}, stored["logs_daily"])

	// the original request retries successfully once storage is healthy
	mem.FailSet = nil
	require.NoError(t, al.AddIndexesToAlias("logs_daily", []string{"logs_2"}))
	indexes, _ = al.GetIndexesForAlias("logs_daily")
	require.Equal(t, []string{"logs_1", "logs_2"}, indexes)
}

func TestAliasList_RemoveRollbackRetryAndEmptyAlias(t *testing.T) {
	mem := withMemMetadata(t)
	al := NewAliasList()

	require.NoError(t, al.AddIndexesToAlias("logs_daily", []string{"logs_1", "logs_2", "logs_3"}))

	mem.FailSet = func(string) error { return errors.New("metadata write failed") }
	err := al.RemoveIndexesFromAlias("logs_daily", []string{"logs_2"})
	require.Error(t, err)

	indexes, ok := al.GetIndexesForAlias("logs_daily")
	require.True(t, ok)
	require.Equal(t, []string{"logs_1", "logs_2", "logs_3"}, indexes)

	stored, err := metadata.Alias.Get()
	require.NoError(t, err)
	require.Equal(t, []string{"logs_1", "logs_2", "logs_3"}, stored["logs_daily"])

	mem.FailSet = nil
	require.NoError(t, al.RemoveIndexesFromAlias("logs_daily", []string{"logs_2"}))
	indexes, _ = al.GetIndexesForAlias("logs_daily")
	require.Equal(t, []string{"logs_1", "logs_3"}, indexes)

	// removing the last member removes the alias, it no longer describes
	// something that cannot be searched
	require.NoError(t, al.RemoveIndexesFromAlias("logs_daily", []string{"logs_1", "logs_3"}))
	_, ok = al.GetIndexesForAlias("logs_daily")
	require.False(t, ok)

	stored, err = metadata.Alias.Get()
	require.NoError(t, err)
	require.NotContains(t, stored, "logs_daily")
}

func TestAliasList_AddDeduplicatesMembers(t *testing.T) {
	withMemMetadata(t)
	al := NewAliasList()

	require.NoError(t, al.AddIndexesToAlias("logs_daily", []string{"logs_1"}))
	require.NoError(t, al.AddIndexesToAlias("logs_daily", []string{"logs_1"}))

	indexes, ok := al.GetIndexesForAlias("logs_daily")
	require.True(t, ok)
	require.Equal(t, []string{"logs_1"}, indexes)
}

func TestAliasList_RemoveIndex(t *testing.T) {
	withMemMetadata(t)
	al := NewAliasList()

	require.NoError(t, al.AddIndexesToAlias("logs_daily", []string{"logs_1", "logs_2"}))
	require.NoError(t, al.AddIndexesToAlias("logs_weekly", []string{"logs_1"}))

	changed, err := al.RemoveIndex("logs_1")
	require.NoError(t, err)
	require.True(t, changed)

	indexes, ok := al.GetIndexesForAlias("logs_daily")
	require.True(t, ok)
	require.Equal(t, []string{"logs_2"}, indexes)

	// alias left without members disappears
	_, ok = al.GetIndexesForAlias("logs_weekly")
	require.False(t, ok)

	stored, err := metadata.Alias.Get()
	require.NoError(t, err)
	require.Equal(t, map[string][]string{"logs_daily": {"logs_2"}}, stored)

	// idempotent: removing an index no alias references changes nothing
	changed, err = al.RemoveIndex("logs_1")
	require.NoError(t, err)
	require.False(t, changed)
}

func TestAliasList_ApplyMembersFull(t *testing.T) {
	mem := withMemMetadata(t)

	liveIndex := "TestAliasList_ApplyMembersFull.index_1"
	index, err := NewIndex(liveIndex, "disk", 1)
	require.NoError(t, err)
	require.NoError(t, StoreIndex(index))
	t.Cleanup(func() {
		ZINC_INDEX_LIST.Delete(liveIndex)
		_ = removeAll(indexDataPath(liveIndex))
	})

	al := NewAliasList()

	// existing index added, unknown skipped, duplicates collapse
	require.NoError(t, al.ApplyMembers(
		map[string][]string{"a1": {liveIndex, "index_that_does_not_exist", liveIndex}},
		nil,
	))
	indexes, ok := al.GetIndexesForAlias("a1")
	require.True(t, ok)
	require.Equal(t, []string{liveIndex}, indexes)

	// add + remove the same member in one request: remove wins, whole batch
	// is one commit
	require.NoError(t, al.ApplyMembers(
		map[string][]string{"a1": {liveIndex}},
		map[string][]string{"a1": {liveIndex}},
	))
	_, ok = al.GetIndexesForAlias("a1")
	require.False(t, ok)

	// persistence failure rolls the whole batch back
	require.NoError(t, al.ApplyMembers(map[string][]string{"a2": {liveIndex}}, nil))
	mem.FailSet = func(string) error { return errors.New("metadata write failed") }
	err = al.ApplyMembers(
		map[string][]string{"a3": {liveIndex}},
		map[string][]string{"a2": {liveIndex}},
	)
	require.Error(t, err)
	indexes, ok = al.GetIndexesForAlias("a2")
	require.True(t, ok)
	require.Equal(t, []string{liveIndex}, indexes)
	_, ok = al.GetIndexesForAlias("a3")
	require.False(t, ok)

	// retry same intent after recovery
	mem.FailSet = nil
	require.NoError(t, al.ApplyMembers(
		map[string][]string{"a3": {liveIndex}},
		map[string][]string{"a2": {liveIndex}},
	))
	_, ok = al.GetIndexesForAlias("a2")
	require.False(t, ok)
	indexes, ok = al.GetIndexesForAlias("a3")
	require.True(t, ok)
	require.Equal(t, []string{liveIndex}, indexes)
}

func TestAliasList_PruneInvalidAliases(t *testing.T) {
	mem := withMemMetadata(t)

	liveIndex := "TestAliasList_PruneInvalidAliases.index_1"
	index, err := NewIndex(liveIndex, "disk", 1)
	require.NoError(t, err)
	require.NoError(t, StoreIndex(index))
	t.Cleanup(func() {
		ZINC_INDEX_LIST.Delete(liveIndex)
		_ = removeAll(indexDataPath(liveIndex))
	})

	al := NewAliasList()
	al.Aliases = map[string][]string{
		"keep":  {liveIndex},
		"gone":  {"index_not_loaded_1"},
		"mixed": {liveIndex, "index_not_loaded_2"},
	}

	require.NoError(t, al.PruneInvalidAliases())

	indexes, ok := al.GetIndexesForAlias("keep")
	require.True(t, ok)
	require.Equal(t, []string{liveIndex}, indexes)
	_, ok = al.GetIndexesForAlias("gone")
	require.False(t, ok)
	indexes, ok = al.GetIndexesForAlias("mixed")
	require.True(t, ok)
	require.Equal(t, []string{liveIndex}, indexes)

	// pruned state is what a restart would load
	stored, err := metadata.Alias.Get()
	require.NoError(t, err)
	require.Equal(t, al.Aliases, stored)

	// if persistence fails at startup the running process still serves the
	// pruned state and reports the error
	al.Aliases = map[string][]string{"gone2": {"index_not_loaded_3"}}
	mem.FailSet = func(string) error { return errors.New("metadata write failed") }
	err = al.PruneInvalidAliases()
	require.Error(t, err)
	_, ok = al.GetIndexesForAlias("gone2")
	require.False(t, ok)
	mem.FailSet = nil
}

func TestAliasList_ConcurrentChangesCommitOneSet(t *testing.T) {
	withMemMetadata(t)
	al := NewAliasList()
	require.NoError(t, al.AddIndexesToAlias("logs_daily", []string{"seed"}))

	const n = 50
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := fmt.Sprintf("logs_%02d", i)
			_ = al.AddIndexesToAlias("logs_daily", []string{name})
		}(i)
	}

	// index deletion detaching a member races with the adds
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = al.RemoveIndex("seed")
	}()

	wg.Wait()

	indexes, ok := al.GetIndexesForAlias("logs_daily")
	require.True(t, ok)
	require.Len(t, indexes, n, "all adds must be committed exactly once, seed removed")
	uniq := make(map[string]struct{}, n)
	for _, name := range indexes {
		require.NotEqual(t, "seed", name)
		uniq[name] = struct{}{}
	}
	require.Len(t, uniq, n, "committed set must contain no duplicates")

	// memory and persisted metadata describe the same committed set
	stored, err := metadata.Alias.Get()
	require.NoError(t, err)
	require.ElementsMatch(t, indexes, stored["logs_daily"])
}
