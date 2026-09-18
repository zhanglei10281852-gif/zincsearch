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

package utils

import (
	"errors"
	"strings"
	"sync"

	"github.com/zincsearch/zincsearch/pkg/metadata/storage"
)

// Sentinel errors mirror pkg/errors (same messages); the fake storage must
// not import pkg/errors because pkg/errors tests import this package.
var (
	ErrMemKeyEmpty    = errors.New("key is be empty")
	ErrMemKeyNotFound = errors.New("key not found")
)

// MemStorage is an in-memory storage.Storager used to inject metadata
// failures in tests. List emulates bbolt bucket semantics: keys are split at
// their last "/" and List(prefix) only returns values stored directly in the
// prefix bucket (nested buckets are excluded), matching the bolt backend.
type MemStorage struct {
	mu         sync.Mutex
	data       map[string][]byte
	FailSet    func(key string) error
	FailDelete func(key string) error
}

func NewMemStorage() *MemStorage {
	return &MemStorage{data: make(map[string][]byte)}
}

var _ storage.Storager = (*MemStorage)(nil)

func (s *MemStorage) Set(key string, value []byte) error {
	if key == "" {
		return errors.ErrKeyEmpty
	}
	if s.FailSet != nil {
		if err := s.FailSet(key); err != nil {
			return err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	buf := make([]byte, len(value))
	copy(buf, value)
	s.data[key] = buf
	return nil
}

func (s *MemStorage) Get(key string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.data[key]
	if !ok {
		return nil, errors.ErrKeyNotFound
	}
	out := make([]byte, len(v))
	copy(out, v)
	return out, nil
}

func (s *MemStorage) Delete(key string) error {
	if key == "" {
		return errors.ErrKeyEmpty
	}
	if s.FailDelete != nil {
		if err := s.FailDelete(key); err != nil {
			return err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
	return nil
}

func (s *MemStorage) List(prefix string, _, _ int) ([][]byte, error) {
	// bbolt resolves the bucket as the part of the key before the last "/",
	// so values nested in deeper buckets are excluded (e.g. templates).
	bucket := prefix
	if i := strings.LastIndex(prefix, "/"); i >= 0 {
		bucket = prefix[:i]
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([][]byte, 0)
	for k, v := range s.data {
		i := strings.LastIndex(k, "/")
		if i < 0 || k[:i] != bucket {
			continue
		}
		buf := make([]byte, len(v))
		copy(buf, v)
		out = append(out, buf)
	}
	return out, nil
}

func (s *MemStorage) Close() error { return nil }
