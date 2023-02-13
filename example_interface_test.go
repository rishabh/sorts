// Copyright 2011 The Go Authors.
// Copyright 2015 Randall Farmer.
// All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sorts_test

import (
	"fmt"

	"github.com/rishabh/sorts"
	"github.com/rishabh/sorts/sortutil"
)

type City struct {
	Name                string
	Latitude, Longitude float32
}

func (c City) String() string { return fmt.Sprintf("%s (%.1f, %.1f)", c.Name, c.Latitude, c.Longitude) }

// ByLatitude implements sort.Interface for []City based on
// the Latitude field, for sorting cities south to north.
type ByLatitude []City

func (a ByLatitude) Len() int      { return len(a) }
func (a ByLatitude) Swap(i, j int) { a[i], a[j] = a[j], a[i] }

// Float32Key and Float32Less make the sort handle the sign bit and sort NaN
// values to the end.  There are also Float64Key and Float64Less, and
// [Type]Key functions for int types.

// Key returns a uint64 that is lower for more southerly latitudes.
func (a ByLatitude) Key(i int) uint64 {
	return sortutil.Float32Key(a[i].Latitude)
}
func (a ByLatitude) Less(i, j int) bool {
	return sortutil.Float32Less(a[i].Latitude, a[j].Latitude)
}

func Example() {
	cities := []City{
		{"Vancouver", 49.3, -123.1},
		{"Tokyo", 35.6, 139.7},
		{"Honolulu", 21.3, -157.8},
		{"Sydney", -33.9, 151.2},
	}

	fmt.Println(cities)
	sorts.ByUint64(ByLatitude(cities))
	fmt.Println(cities)

	// Output:
	// [Vancouver (49.3, -123.1) Tokyo (35.6, 139.7) Honolulu (21.3, -157.8) Sydney (-33.9, 151.2)]
	// [Sydney (-33.9, 151.2) Honolulu (21.3, -157.8) Tokyo (35.6, 139.7) Vancouver (49.3, -123.1)]
}
