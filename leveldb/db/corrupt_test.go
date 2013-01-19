// Copyright (c) 2013, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// This LevelDB Go implementation is based on LevelDB C++ implementation.
// Which contains the following header:
//   Copyright (c) 2011 The LevelDB Authors. All rights reserved.
//   Use of this source code is governed by a BSD-style license that can be
//   found in the LEVELDBCPP_LICENSE file. See the LEVELDBCPP_AUTHORS file
//   for names of contributors.

package db

import (
	"bytes"
	"fmt"
	"io"
	"leveldb/cache"
	"leveldb/descriptor"
	"leveldb/log"
	"leveldb/opt"
	"testing"
)

const ctValSize = 1000

type dbCorruptHarness struct {
	dbHarness
}

func newDbCorruptHarness(t *testing.T) *dbCorruptHarness {
	h := new(dbCorruptHarness)
	h.init(t, &opt.Options{
		Flag:       opt.OFCreateIfMissing,
		BlockCache: cache.NewLRUCache(100),
	})
	return h
}

func (h *dbCorruptHarness) recover() {
	p := &h.dbHarness
	t := p.t

	var err error
	p.db, err = Recover(h.desc, h.o)
	if err != nil {
		t.Fatal("Repair: got error: ", err)
	}
}

func (h *dbCorruptHarness) build(n int) {
	p := &h.dbHarness
	t := p.t
	db := p.db

	batch := new(Batch)
	for i := 0; i < n; i++ {
		batch.Reset()
		batch.Put(tkey(i), tval(i, ctValSize))
		err := db.Write(batch, p.wo)
		if err != nil {
			t.Fatal("write error: ", err)
		}
	}
}

func (h *dbCorruptHarness) corrupt(ft descriptor.FileType, offset, n int) {
	p := &h.dbHarness
	t := p.t

	var file descriptor.File
	for _, f := range p.desc.GetFiles(ft) {
		if file == nil || f.Number() > file.Number() {
			file = f
		}
	}
	if file == nil {
		t.Fatalf("no such file with type %q", ft)
	}

	r, err := file.Open()
	if err != nil {
		t.Fatal("cannot open file: ", err)
	}
	x, err := file.Size()
	if err != nil {
		t.Fatal("cannot query file size: ", err)
	}
	m := int(x)

	if offset < 0 {
		if -offset > m {
			offset = 0
		} else {
			offset = m + offset
		}
	}
	if offset > m {
		offset = m
	}
	if offset+n > m {
		n = m - offset
	}

	buf := make([]byte, m)
	_, err = io.ReadFull(r, buf)
	if err != nil {
		t.Fatal("cannot read file: ", err)
	}
	r.Close()

	for i := 0; i < n; i++ {
		buf[offset+i] ^= 0x80
	}

	err = file.Remove()
	if err != nil {
		t.Fatal("cannot remove old file: ", err)
	}
	w, err := file.Create()
	if err != nil {
		t.Fatal("cannot create new file: ", err)
	}
	_, err = w.Write(buf)
	if err != nil {
		t.Fatal("cannot write new file: ", err)
	}
	w.Close()
}

func (h *dbCorruptHarness) check(min, max int) {
	p := &h.dbHarness
	t := p.t
	db := p.db

	var n, badk, badv, missed, good int
	iter := db.NewIterator(p.ro)
	for iter.Next() {
		k := 0
		fmt.Sscanf(string(iter.Key()), "%d", &k)
		if k < n {
			badk++
			continue
		}
		missed += k - n
		n = k + 1
		if !bytes.Equal(iter.Value(), tval(k, ctValSize)) {
			badv++
		} else {
			good++
		}
	}

	t.Logf("want=%d..%d got=%d badkeys=%d badvalues=%d missed=%d",
		min, max, good, badk, badv, missed)
	if good < min || good > max {
		t.Errorf("good entries number not in range")
	}
}

func TestCorruptDB_Log(t *testing.T) {
	h := newDbCorruptHarness(t)

	h.build(100)
	h.check(100, 100)
	h.close()
	h.corrupt(descriptor.TypeLog, 19, 1)
	h.corrupt(descriptor.TypeLog, log.BlockSize+1000, 1)

	h.open()
	h.check(36, 36)

	h.close()
}

func TestCorruptDB_Table(t *testing.T) {
	h := newDbCorruptHarness(t)

	h.build(100)
	h.compactMem()
	h.compactRangeAt(0, "", "")
	h.compactRangeAt(1, "", "")
	h.close()
	h.corrupt(descriptor.TypeTable, 100, 1)

	h.open()
	h.check(99, 99)

	h.close()
}

func TestCorruptDB_TableIndex(t *testing.T) {
	h := newDbCorruptHarness(t)

	h.build(10000)
	h.compactMem()
	h.close()
	h.corrupt(descriptor.TypeTable, -2000, 500)

	h.open()
	h.check(5000, 9999)

	h.close()
}

func TestCorruptDB_MissingManifest(t *testing.T) {
	h := newDbCorruptHarness(t)

	h.build(1000)
	h.compactMem()
	h.build(1000)
	h.compactMem()
	h.build(1000)
	h.compactMem()
	h.build(1000)
	h.compactMem()
	h.close()

	h.recover()
	h.check(1000, 1000)
	h.build(1000)
	h.compactMem()
	h.close()

	h.recover()
	h.check(1000, 1000)

	h.close()
}

func TestCorruptDB_SequenceNumberRecovery(t *testing.T) {
	h := newDbCorruptHarness(t)

	h.put("foo", "v1")
	h.put("foo", "v2")
	h.put("foo", "v3")
	h.put("foo", "v4")
	h.put("foo", "v5")
	h.close()

	h.recover()
	h.getVal("foo", "v5")
	h.put("foo", "v6")
	h.getVal("foo", "v6")

	h.reopen()
	h.getVal("foo", "v6")

	h.close()
}

func TestCorruptDB_SequenceNumberRecoveryTable(t *testing.T) {
	h := newDbCorruptHarness(t)

	h.put("foo", "v1")
	h.put("foo", "v2")
	h.put("foo", "v3")
	h.compactMem()
	h.put("foo", "v4")
	h.put("foo", "v5")
	h.compactMem()
	h.close()

	h.recover()
	h.getVal("foo", "v5")
	h.put("foo", "v6")
	h.getVal("foo", "v6")

	h.reopen()
	h.getVal("foo", "v6")

	h.close()
}

func TestCorruptDB_CorruptedManifest(t *testing.T) {
	h := newDbCorruptHarness(t)

	h.put("foo", "hello")
	h.compactMem()
	h.compactRange("", "")
	h.close()
	h.corrupt(descriptor.TypeManifest, 0, 1000)
	h.openAssert(false)

	h.recover()
	h.getVal("foo", "hello")

	h.close()
}

func TestCorruptDB_CompactionInputError(t *testing.T) {
	h := newDbCorruptHarness(t)

	h.build(10)
	h.compactMem()
	h.close()
	h.corrupt(descriptor.TypeTable, 100, 1)

	h.open()
	h.check(9, 9)

	h.build(10000)
	h.check(10000, 10000)

	h.close()
}

func TestCorruptDB_UnrelatedKeys(t *testing.T) {
	h := newDbCorruptHarness(t)

	h.build(10)
	h.compactMem()
	h.close()
	h.corrupt(descriptor.TypeTable, 100, 1)

	h.open()
	h.put(string(tkey(1000)), string(tval(1000, ctValSize)))
	h.getVal(string(tkey(1000)), string(tval(1000, ctValSize)))
	h.compactMem()
	h.getVal(string(tkey(1000)), string(tval(1000, ctValSize)))

	h.close()
}