// Copyright 2014-5 Randall Farmer. All rights reserved.

// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package radixsort

import (
	"bytes"
)

const radix = 8
const mask = (1 << radix) - 1

// qSortCutoff is when we bail out to a quicksort. It's changed to 1 for
// certain tests so we can more easily exercise the radix sorting.  This was
// around the break-even point in some sloppy tests.
var qSortCutoff = 1 << 7

const keyPanicMessage = "sort failed: Key and Less aren't consistent with each other"
const keyNumberHelp = " (the [NumberType]Key functions like IntKey may help resolve this)"
const panicMessage = "sort failed: could be a data race, a radixsort bug, or a subtle bug in the interface implementation"

// maxRadixDepth limits how deeply the radix part of string sorts can
// recurse before we bail to quicksort.  Each recursion uses 2KB stack.
const maxRadixDepth = 32

// task describes a range of data to be sorted and additional
// information the sorter needs: bitshift in a numeric sort, byte offset in
// a string sort, or maximum depth (expressed as -maxDepth-1) for a
// quicksort.
type task struct{ offs, pos, end int }

// ByNumber sorts data by a uint64 key. To use it with signed or
// floating-point data, use helper functions for the corresponding type,
// like IntKey or Float32Key and Float32Less.
func ByNumber(data NumberInterface) {
	l := data.Len()
	shift := guessIntShift(data, l)
	parallelSort(data, radixSortUint64, task{offs: int(shift), end: l})

	// check results!
	for i := 1; i < l; i++ {
		if data.Less(i, i-1) {
			if data.Key(i) > data.Key(i-1) {
				panic(keyPanicMessage + keyNumberHelp)
			}
			panic(panicMessage)
		}
	}
}

// ByString sorts data by a string key.
func ByString(data StringInterface) {
	l := data.Len()
	parallelSort(data, radixSortString, task{end: l})

	// check results!
	for i := 1; i < l; i++ {
		if data.Less(i, i-1) {
			if data.Key(i) > data.Key(i-1) {
				panic(keyPanicMessage)
			}
			panic(panicMessage)
		}
	}
}

// ByBytes sorts data by a []byte key.
func ByBytes(data BytesInterface) {
	l := data.Len()
	parallelSort(data, radixSortBytes, task{end: l})

	// check results!
	for i := 1; i < l; i++ {
		if data.Less(i, i-1) {
			if bytes.Compare(data.Key(i), data.Key(i-1)) > 0 {
				panic(keyPanicMessage)
			}
			panic(panicMessage)
		}
	}
}

// guessIntShift saves a pass when the data is distributed roughly uniformly
// in a small range (think shuffled indices into a small array), and rarely
// hurts much otherwise: either it just returns 64-radix quickly, or it
// returns too small a shift and the sort notices after one useless counting
// pass.
func guessIntShift(data NumberInterface, l int) uint {
	if l < qSortCutoff {
		return 64 - radix
	}
	step := l >> 5
	if l > 1<<16 {
		step = l >> 8
	}
	if step == 0 { // only for tests w/qSortCutoff lowered
		step = 1
	}
	min := data.Key(l - 1)
	max := min
	for i := 0; i < l; i += step {
		k := data.Key(i)
		if k < min {
			min = k
		}
		if k > max {
			max = k
		}
	}
	diff := min ^ max
	log2diff := 0
	for diff != 0 {
		log2diff++
		diff >>= 1
	}
	shiftGuess := log2diff - radix
	if shiftGuess < 0 {
		return 0
	}
	return uint(shiftGuess)
}

/*
Thanks to (and please refer to):

Victor J. Duvanenko, "Parallel In-Place Radix Sort Simplified", 2011, at
http://www.drdobbs.com/parallel/parallel-in-place-radix-sort-simplified/229000734
for lots of practical discussion of performance

Michael Herf, "Radix Tricks", 2001, at
http://stereopsis.com/radix.html
for the idea for Float32Key()/Float64Key() (via Pierre Tardiman, "Radix Sort
Revisited", 2000, at http://codercorner.com/RadixSortRevisited.htm) and more
performance talk.

A handy slide deck summarizing Robert Sedgewick and Kevin Wayne's Algorithms
on string sorts:
http://algs4.cs.princeton.edu/lectures/51StringSorts.pdf
for a grounding in string sorts and pointer to American flag sort

McIlroy, Bostic, and McIlroy, "Engineering Radix Sort", 1993 at
http://citeseerx.ist.psu.edu/viewdoc/summary?doi=10.1.1.22.6990
for laying out American flag sort

- We're not using American flag sort's trick of keeping our own stack. It
  might help on some data, but just bailing to qsort after 32 bytes is
  enough to keep stack use from exploding.

- I suspect the quicksort phase could be sped up, especially for strings.
  If you collected the next, say, eight bytes of each string in an array,
  sorted those, and only compared full strings as a tiebreaker, you could
  likely avoid following a lot of pointers and use cache better. That's a
  lot of work and a lot of code, though.

- I'm sure with a radically different approach--like with a type like this:
  type Index struct { Indices, Keys uint64 }
  you could do a bunch of other cool things.

*/

// All three radixSort functions below do a counting pass and a swapping
// pass, then recurse.  They fall back to comparison sort for small buckets
// and equal ranges, and the int sort tries to skip bits that are identical
// across the whole range being sorted.

func radixSortUint64(dataI interface{}, t task, sortRange func(task)) {
	data := dataI.(NumberInterface)
	shift, a, b := uint(t.offs), t.pos, t.end
	if b-a < qSortCutoff {
		qSort(data, a, b)
		return
	}

	// use a single pass over the keys to bucket data and find min/max
	// (for skipping over bits that are always identical)
	var bucketStarts, bucketEnds [1 << radix]int
	min := data.Key(a)
	max := min
	for i := a; i < b; i++ {
		k := data.Key(i)
		bucketStarts[(k>>shift)&mask]++
		if k < min {
			min = k
		}
		if k > max {
			max = k
		}
	}

	// skip past common prefixes, bail if all keys equal
	diff := min ^ max
	if diff == 0 {
		return
	}
	if diff>>shift == 0 || diff>>(shift+radix) != 0 {
		// find highest 1 bit in diff
		log2diff := 0
		for diff != 0 {
			log2diff++
			diff >>= 1
		}
		nextShift := log2diff - radix
		if nextShift < 0 {
			nextShift = 0
		}
		sortRange(task{int(nextShift), a, b})
		return
	}

	pos := a
	for i, c := range bucketStarts {
		bucketStarts[i] = pos
		pos += c
		bucketEnds[i] = pos
	}

	for curBucket, bucketEnd := range bucketEnds {
		i := bucketStarts[curBucket]
		for i < bucketEnd {
			destBucket := (data.Key(i) >> shift) & mask
			if destBucket == uint64(curBucket) {
				i++
				bucketStarts[destBucket]++
				continue
			}
			data.Swap(i, bucketStarts[destBucket])
			bucketStarts[destBucket]++
		}
	}

	if shift == 0 {
		// each bucket is a unique key
		return
	}

	nextShift := shift - radix
	if shift < radix {
		nextShift = 0
	}
	pos = a
	for _, end := range bucketEnds {
		if end > pos+1 {
			sortRange(task{int(nextShift), pos, end})
		}
		pos = end
	}
}

func radixSortString(dataI interface{}, t task, sortRange func(task)) {
	data := dataI.(StringInterface)
	offset, a, b := t.offs, t.pos, t.end
	if offset < 0 {
		// in a parallel quicksort of items w/long common key prefix
		quickSortWorker(data, t, sortRange)
		return
	}
	if b-a < qSortCutoff {
		qSort(data, a, b)
		return
	}
	if offset == maxRadixDepth {
		qSortPar(data, t, sortRange)
		return
	}

	// swap too-short strings to start and count bucket sizes
	bucketStarts, bucketEnds := [256]int{}, [256]int{}
	for i := a; i < b; i++ {
		k := data.Key(i)
		if len(k) <= offset {
			// swap too-short strings to start
			data.Swap(a, i)
			a++
			continue
		}
		bucketStarts[k[offset]]++
	}

	pos := a
	for i, c := range bucketStarts {
		bucketStarts[i] = pos
		pos += c
		bucketEnds[i] = pos
		if bucketStarts[i] == a && bucketEnds[i] == b {
			// everything was in the same bucket
			sortRange(task{offset + 1, a, b})
			return
		}
	}

	i := a
	for curBucket, bucketEnd := range bucketEnds {
		start := i
		i = bucketStarts[curBucket]
		for i < bucketEnd {
			destBucket := data.Key(i)[offset]
			if destBucket == byte(curBucket) {
				i++
				bucketStarts[destBucket]++
				continue
			}
			data.Swap(i, bucketStarts[destBucket])
			bucketStarts[destBucket]++
		}
		if i > start+1 {
			sortRange(task{offset + 1, start, i})
		}
	}
}

func radixSortBytes(dataI interface{}, t task, sortRange func(task)) {
	data := dataI.(BytesInterface)
	offset, a, b := t.offs, t.pos, t.end
	if offset < 0 {
		// in a parallel quicksort of items w/long common key prefix
		quickSortWorker(data, t, sortRange)
		return
	}
	if b-a < qSortCutoff {
		qSort(data, a, b)
		return
	}
	if offset == maxRadixDepth {
		qSortPar(data, t, sortRange)
		return
	}

	// swap too-short strings to start and count bucket sizes
	bucketStarts, bucketEnds := [256]int{}, [256]int{}
	for i := a; i < b; i++ {
		k := data.Key(i)
		if len(k) <= offset {
			// swap too-short strings to start
			data.Swap(a, i)
			a++
			continue
		}
		bucketStarts[k[offset]]++
	}

	pos := a
	for i, c := range bucketStarts {
		bucketStarts[i] = pos
		pos += c
		bucketEnds[i] = pos
		if bucketStarts[i] == a && bucketEnds[i] == b {
			// everything was in the same bucket
			sortRange(task{offset + 1, a, b})
			return
		}
	}

	i := a
	for curBucket, bucketEnd := range bucketEnds {
		start := i
		i = bucketStarts[curBucket]
		for i < bucketEnd {
			destBucket := data.Key(i)[offset]
			if destBucket == byte(curBucket) {
				i++
				bucketStarts[destBucket]++
				continue
			}
			data.Swap(i, bucketStarts[destBucket])
			bucketStarts[destBucket]++
		}
		if i > start+1 {
			sortRange(task{offset + 1, start, i})
		}
	}
}
