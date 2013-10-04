// Copyright (c) 2012, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package leveldb

import (
	"sync"

	"github.com/conformal/goleveldb/leveldb/cache"
	"github.com/conformal/goleveldb/leveldb/comparer"
	"github.com/conformal/goleveldb/leveldb/filter"
	"github.com/conformal/goleveldb/leveldb/opt"
)

type iOptions struct {
	opt.Options
	s  *session
	mu sync.Mutex
}

func newIOptions(s *session, o opt.Options) *iOptions {
	p := &iOptions{Options: o, s: s}
	p.sanitize()
	return p
}

func (o *iOptions) sanitize() {
	if p := o.GetBlockCache(); p == nil {
		o.Options.SetBlockCache(cache.NewLRUCache(opt.DefaultBlockCacheSize))
	}

	for _, p := range o.GetAltFilters() {
		o.InsertAltFilter(p)
	}

	if p := o.GetFilter(); p != nil {
		o.SetFilter(p)
	}
}

func (o *iOptions) GetComparer() comparer.Comparer {
	return o.s.cmp
}

func (o *iOptions) SetComparer(cmp comparer.Comparer) error {
	return opt.ErrNotAllowed
}

func (o *iOptions) SetMaxOpenFiles(max int) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	err := o.Options.SetMaxOpenFiles(max)
	if err != nil {
		return err
	}
	o.s.tops.cache.SetCapacity(max)
	return nil
}

func (o *iOptions) SetBlockCache(cache cache.Cache) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	oldcache := o.Options.GetBlockCache()
	err := o.Options.SetBlockCache(cache)
	if err != nil {
		return err
	}
	if oldcache != nil {
		oldcache.Purge(nil)
	}
	o.s.tops.cache.Purge(nil)
	return nil
}

func (o *iOptions) SetFilter(f filter.Filter) error {
	if f != nil {
		f = iFilter{f}
	}
	return o.Options.SetFilter(f)
}

func (o *iOptions) InsertAltFilter(f filter.Filter) error {
	if f == nil {
		return opt.ErrInvalid
	}
	return o.Options.InsertAltFilter(iFilter{f})
}
