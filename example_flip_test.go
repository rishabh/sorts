// Copyright 2013 The Go Authors.
// Copyright 2015 Randall Farmer.
// All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sorts

import (
	"fmt"
)

func Example_flip() {
	scores := []int{39, 492, 4912, 39, -10, 4, 92}
	data := IntSlice(scores)
	data.Sort()
	Flip(data) // high scores first
	fmt.Println(scores)
	// Output: [4912 492 92 39 39 4 -10]
}
